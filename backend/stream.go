package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// WebSocket close codes used when ending a client connection.
const (
	closeGoingAway   = 1001
	closeInternalErr = 1011
)

// ErrCapacity is returned when MAX_STREAMS FFmpeg processes are already running.
var ErrCapacity = errors.New("server is at capacity, try again later")

// ---------------------------------------------------------------- Client

// Client is one WebSocket viewer subscribed to a Stream.
type Client struct {
	stream *Stream
	send   chan []byte
	done   chan struct{}
	once   sync.Once
	code   int
	reason string
	paused atomic.Bool
}

func newClient() *Client {
	return &Client{send: make(chan []byte, 64), done: make(chan struct{})}
}

// Close asks the connection handler to send a close frame and disconnect.
func (c *Client) Close(code int, reason string) {
	c.once.Do(func() {
		c.code, c.reason = code, reason
		close(c.done)
	})
}

// ---------------------------------------------------------------- Manager

// Manager owns all running streams. Viewers of the same URL share one FFmpeg process.
type Manager struct {
	cfg     Config
	ctx     context.Context
	wg      sync.WaitGroup
	mu      sync.Mutex
	streams map[string]*Stream
}

type Stats struct {
	Streams    int `json:"streams"`
	Clients    int `json:"clients"`
	MaxStreams int `json:"maxStreams"`
}

func NewManager(ctx context.Context, cfg Config) *Manager {
	return &Manager{cfg: cfg, ctx: ctx, streams: make(map[string]*Stream)}
}

// Subscribe attaches a new viewer to the stream for rawURL, starting FFmpeg if needed.
func (m *Manager) Subscribe(rawURL string) (*Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.streams[rawURL]
	if !ok {
		if len(m.streams) >= m.cfg.MaxStreams {
			return nil, ErrCapacity
		}
		ctx, cancel := context.WithCancel(m.ctx)
		s = &Stream{
			rawURL:  rawURL,
			mgr:     m,
			ctx:     ctx,
			cancel:  cancel,
			clients: make(map[*Client]struct{}),
		}
		m.streams[rawURL] = s
		m.wg.Add(1)
		go s.run()
	}
	c := newClient()
	c.stream = s
	s.addClient(c)
	return c, nil
}

func (m *Manager) Unsubscribe(c *Client) {
	if c.stream != nil {
		c.stream.removeClient(c)
	}
}

func (m *Manager) Stats() Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := Stats{Streams: len(m.streams), MaxStreams: m.cfg.MaxStreams}
	for _, s := range m.streams {
		s.mu.RLock()
		st.Clients += len(s.clients)
		s.mu.RUnlock()
	}
	return st
}

// Wait blocks until all FFmpeg processes have exited (or timeout).
func (m *Manager) Wait(timeout time.Duration) {
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func (m *Manager) remove(s *Stream) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.streams[s.rawURL] == s {
		delete(m.streams, s.rawURL)
	}
}

// reapIfIdle stops a stream that still has no viewers after the grace period.
func (m *Manager) reapIfIdle(s *Stream) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.clients) > 0 || m.streams[s.rawURL] != s {
		return
	}
	delete(m.streams, s.rawURL)
	log.Printf("stream %s: no viewers, stopping ffmpeg", redact(s.rawURL))
	s.cancel()
}

// ---------------------------------------------------------------- Stream

// Stream is one FFmpeg process fanned out to many WebSocket clients.
type Stream struct {
	rawURL string
	mgr    *Manager
	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.RWMutex
	clients   map[*Client]struct{}
	idleTimer *time.Timer
}

func (s *Stream) addClient(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
	s.clients[c] = struct{}{}
}

func (s *Stream) removeClient(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.clients[c]; !ok {
		return
	}
	delete(s.clients, c)
	if len(s.clients) == 0 {
		s.idleTimer = time.AfterFunc(s.mgr.cfg.IdleGrace, func() { s.mgr.reapIfIdle(s) })
	}
}

func (s *Stream) closeAll(code int, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		c.Close(code, reason)
	}
	s.clients = make(map[*Client]struct{})
}

// broadcast sends a chunk to every unpaused client. Slow clients drop chunks
// instead of blocking everyone else (video recovers at the next keyframe).
func (s *Stream) broadcast(chunk []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.clients) == 0 {
		return
	}
	data := make([]byte, len(chunk))
	copy(data, chunk)
	for c := range s.clients {
		if c.paused.Load() {
			continue
		}
		select {
		case c.send <- data:
		default: // client too slow, drop
		}
	}
}

// run supervises FFmpeg: restarts it with backoff if it drops, gives up after repeated failures.
func (s *Stream) run() {
	defer s.mgr.wg.Done()

	code, reason := closeGoingAway, "stream stopped"
	failures, everWorked := 0, false

	for s.ctx.Err() == nil {
		started := time.Now()
		gotData, err := s.runFFmpeg()
		if s.ctx.Err() != nil {
			break
		}
		everWorked = everWorked || gotData
		if time.Since(started) > 15*time.Second {
			failures = 0
		}
		failures++
		msg := sanitize(err.Error(), s.rawURL)
		log.Printf("stream %s: ffmpeg stopped (attempt %d): %s", redact(s.rawURL), failures, msg)

		limit := 5
		if !everWorked {
			limit = 2 // fail fast on bad URLs / unreachable hosts
		}
		if failures >= limit {
			code, reason = closeInternalErr, "could not read stream: "+shortMsg(msg)
			break
		}
		select {
		case <-s.ctx.Done():
		case <-time.After(time.Duration(failures) * 2 * time.Second):
		}
	}

	s.cancel()
	s.mgr.remove(s)
	s.closeAll(code, reason)
}

// runFFmpeg runs one FFmpeg process until it exits. Returns whether any video was produced.
func (s *Stream) runFFmpeg() (bool, error) {
	cfg := s.mgr.cfg
	runCtx, runCancel := context.WithCancel(s.ctx)
	defer runCancel()

	cmd := exec.CommandContext(runCtx, cfg.FFmpegPath, ffmpegArgs(cfg, s.rawURL)...)
	cmd.WaitDelay = 2 * time.Second
	stderr := &tailBuffer{max: 1024}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false, err
	}
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("start ffmpeg: %w", err)
	}
	log.Printf("stream %s: ffmpeg started (pid %d)", redact(s.rawURL), cmd.Process.Pid)

	// Watchdog: kill FFmpeg if it hangs without producing data.
	var last atomic.Int64
	var stalled atomic.Bool
	last.Store(time.Now().UnixNano())
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-t.C:
				if time.Since(time.Unix(0, last.Load())) > cfg.StallTimeout {
					stalled.Store(true)
					runCancel()
					return
				}
			}
		}
	}()

	buf := make([]byte, 188*100) // multiple of the MPEG-TS packet size
	gotData := false
	for {
		n, rerr := stdout.Read(buf)
		if n > 0 {
			gotData = true
			last.Store(time.Now().UnixNano())
			s.broadcast(buf[:n])
		}
		if rerr != nil {
			break
		}
	}
	waitErr := cmd.Wait()

	switch {
	case stalled.Load():
		return gotData, errors.New("no data received from stream (timed out)")
	case waitErr != nil:
		if out := strings.TrimSpace(stderr.String()); out != "" {
			log.Printf("stream %s: ffmpeg output:\n%s", redact(s.rawURL), sanitize(out, s.rawURL))
		}
		return gotData, fmt.Errorf("%v: %s", waitErr, firstLine(stderr.String()))
	default:
		return gotData, errors.New("stream ended")
	}
}

// ffmpegArgs converts RTSP into MPEG-TS/MPEG1 video, which JSMpeg plays in the browser.
func ffmpegArgs(cfg Config, rawURL string) []string {
	w := cfg.VideoWidth - cfg.VideoWidth%2
	return []string{
		"-hide_banner", "-loglevel", "error",
		"-rtsp_transport", "tcp",
		"-timeout", "10000000", // socket timeout, microseconds
		"-fflags", "nobuffer", "-flags", "low_delay",
		"-i", rawURL,
		"-an",
		"-vf", fmt.Sprintf("scale=%d:-2", w),
		"-r", strconv.Itoa(cfg.FPS),
		"-c:v", "mpeg1video",
		"-b:v", cfg.VideoBitrate,
		"-bf", "0",
		"-g", strconv.Itoa(cfg.FPS * 2), // keyframe every ~2s so new viewers can join fast
		"-f", "mpegts",
		"pipe:1",
	}
}

// ---------------------------------------------------------------- helpers

// tailBuffer keeps only the last `max` bytes written (for FFmpeg's stderr).
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// firstLine returns the first line of FFmpeg's output: usually the root cause,
// while later lines are consequences.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// redact hides credentials in a URL for logging.
func redact(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Redacted()
	}
	return "<invalid url>"
}

// sanitize removes the URL and any password from text that may be shown to users.
func sanitize(msg, raw string) string {
	msg = strings.ReplaceAll(msg, raw, redact(raw))
	if u, err := url.Parse(raw); err == nil && u.User != nil {
		if pw, ok := u.User.Password(); ok && pw != "" {
			msg = strings.ReplaceAll(msg, pw, "***")
		}
	}
	return msg
}

func shortMsg(s string) string {
	if len(s) > 80 {
		return s[:80]
	}
	return s
}