package main

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime settings, loaded from environment variables.
type Config struct {
	Port           string
	FFmpegPath     string
	AllowedOrigins []string      // empty = allow any origin (dev only)
	AllowedHosts   []string      // empty = allow any RTSP host
	MaxStreams     int           // max concurrent FFmpeg processes
	VideoWidth     int           // output width in px (height keeps aspect ratio)
	FPS            int           // output frame rate
	VideoBitrate   string        // e.g. "600k"
	IdleGrace      time.Duration // keep FFmpeg alive this long after last viewer leaves
	StallTimeout   time.Duration // kill FFmpeg if no data arrives for this long
}

func LoadConfig() Config {
	return Config{
		Port:           env("PORT", "8080"),
		FFmpegPath:     env("FFMPEG_PATH", "ffmpeg"),
		AllowedOrigins: csv(os.Getenv("ALLOWED_ORIGINS")),
		AllowedHosts:   csv(os.Getenv("ALLOWED_HOSTS")),
		MaxStreams:     envInt("MAX_STREAMS", 6),
		VideoWidth:     envInt("VIDEO_WIDTH", 640),
		FPS:            validFPS(envInt("VIDEO_FPS", 25)),
		VideoBitrate:   env("VIDEO_BITRATE", "600k"),
		IdleGrace:      time.Duration(envInt("IDLE_GRACE_SECONDS", 10)) * time.Second,
		StallTimeout:   time.Duration(envInt("STALL_TIMEOUT_SECONDS", 15)) * time.Second,
	}
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func csv(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// validFPS enforces the frame rates MPEG-1 video supports; others make FFmpeg fail.
func validFPS(n int) int {
	switch n {
	case 24, 25, 30, 50, 60:
		return n
	}
	log.Printf("VIDEO_FPS=%d is not supported by MPEG-1 (use 24, 25, 30, 50 or 60); using 25", n)
	return 25
}