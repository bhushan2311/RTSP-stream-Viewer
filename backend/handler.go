package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 30 * time.Second
	maxURLLen  = 2048

	closePolicyViolation = 1008
	closeTryAgainLater   = 1013
)

type Handler struct {
	cfg      Config
	mgr      *Manager
	upgrader websocket.Upgrader
}

func NewHandler(cfg Config, mgr *Manager) *Handler {
	h := &Handler{cfg: cfg, mgr: mgr}
	h.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 32 * 1024,
		CheckOrigin:     h.checkOrigin,
	}
	return h
}

func (h *Handler) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if len(h.cfg.AllowedOrigins) == 0 || origin == "" {
		return true
	}
	for _, a := range h.cfg.AllowedOrigins {
		if a == "*" || strings.EqualFold(strings.TrimRight(a, "/"), strings.TrimRight(origin, "/")) {
			return true
		}
	}
	return false
}

// Health reports basic load numbers.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.mgr.Stats())
}

// ServeWS handles GET /ws?url=rtsp://...
// Binary messages = MPEG-TS video. Client may send {"action":"pause"|"play"}.
// Errors are reported through the WebSocket close code and reason.
func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	if err := validateRTSPURL(raw, h.cfg.AllowedHosts); err != nil {
		writeClose(conn, closePolicyViolation, err.Error())
		return
	}

	client, err := h.mgr.Subscribe(raw)
	if err != nil {
		writeClose(conn, closeTryAgainLater, err.Error())
		return
	}
	defer h.mgr.Unsubscribe(client)

	gone := make(chan struct{})
	go readLoop(conn, client, gone)

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case chunk := <-client.send:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.BinaryMessage, chunk); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-client.done:
			writeClose(conn, client.code, client.reason)
			return
		case <-gone:
			return
		}
	}
}

// readLoop processes control messages and detects disconnects.
func readLoop(conn *websocket.Conn, c *Client, gone chan<- struct{}) {
	defer close(gone)
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if mt != websocket.TextMessage {
			continue
		}
		var cmd struct {
			Action string `json:"action"`
		}
		if json.Unmarshal(msg, &cmd) != nil {
			continue
		}
		switch cmd.Action {
		case "pause":
			c.paused.Store(true)
		case "play":
			c.paused.Store(false)
		}
	}
}

func writeClose(conn *websocket.Conn, code int, reason string) {
	if len(reason) > 120 {
		reason = reason[:120]
	}
	reason = strings.ToValidUTF8(reason, "")
	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason), time.Now().Add(writeWait))
}

func validateRTSPURL(raw string, allowedHosts []string) error {
	if raw == "" {
		return errors.New("missing url parameter")
	}
	if len(raw) > maxURLLen {
		return errors.New("url too long")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid url")
	}
	if u.Scheme != "rtsp" && u.Scheme != "rtsps" {
		return errors.New("url must start with rtsp:// or rtsps://")
	}
	if u.Hostname() == "" {
		return errors.New("url has no host")
	}
	if len(allowedHosts) > 0 {
		for _, h := range allowedHosts {
			if strings.EqualFold(h, u.Hostname()) {
				return nil
			}
		}
		return errors.New("host not allowed")
	}
	return nil
}
