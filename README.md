# RTSP Stream Viewer

A web application that lets you add RTSP stream URLs and watch the live streams in your browser, several at once in a responsive grid. Built with a **React + TypeScript** frontend and a **Go** backend that uses **FFmpeg** to convert RTSP into a browser-friendly format and delivers it over **WebSockets**.

<img width="1917" height="958" alt="Screenshot 2026-10-07 114321" src="https://github.com/user-attachments/assets/a1a85c41-5d08-454a-b04e-b851c763eed5" />


| | |
|---|---|
| **Live demo (frontend)** | https://rtsp-stream-viewer-nine.vercel.app |
| **Backend (Render)** | https://rtsp-stream-viewer-0hg9.onrender.com (health check: `/api/health`) |
| **Source code** | https://github.com/bhushan2311/RTSP-stream-Viewer |
| **Login required** | No, the app has no accounts or credentials |

> **Free hosting:** the backend runs on a free plan that sleeps after about 15 minutes without traffic. The first request after that can take around a minute; the UI shows "Waiting for the server…" and retries automatically.

---

## Table of contents
1. [Try the live demo](#try-the-live-demo)
2. [Features](#features)
3. [How it works](#how-it-works)
4. [Tech stack](#tech-stack)
5. [Project structure](#project-structure)
6. [Run locally](#run-locally)
7. [Configuration](#configuration)
8. [Backend API](#backend-api)
9. [Error handling](#error-handling)
10. [Performance and scalability](#performance-and-scalability)
11. [Troubleshooting](#troubleshooting)
12. [Known limitations and next steps](#known-limitations-and-next-steps)

---

## Try the live demo

1. Open https://rtsp-stream-viewer-nine.vercel.app (the first load can take about a minute).
2. Paste any **publicly reachable RTSP URL** (`rtsp://...`) and click **Add stream**.

**If a stream does not play on the live site, this is usually why:**
- **The address is not reachable from the internet.** The backend runs in the cloud, so it can only open streams that are publicly accessible. Addresses such as `rtsp://127.0.0.1:...`, `rtsp://localhost:...` or `rtsp://192.168.x.x:...` point to the cloud server itself or to a private network, not to your computer, so they fail. They work when you [run the project locally](#run-locally).
- **The public test stream is offline or blocked.** Free public RTSP test streams are unreliable: they go offline, change address, or refuse access (for example "403 Forbidden" or "host not found"). That is a problem with the stream, not the app. The tile shows the reason and a **Try again** button.
- **The stream needs a login.** Include the credentials in the URL (`rtsp://user:password@host/path`); the stream must still be publicly reachable.
- **Firewall or closed port.** The stream's RTSP port (usually 554 or 8554) must be open to the internet. The backend connects over TCP.
- **The server is small.** The demo runs on a free plan with very little CPU, so only a few streams can run at once ("server is at capacity" appears beyond that) and video may stutter with several streams.

**To see the app working without a public stream,** run it locally with the included MediaMTX + FFmpeg test setup (see [Run locally](#run-locally)); the screenshot above shows that setup.

---

## Features
- Add an RTSP stream by URL, with validation (must start with `rtsp://` or `rtsps://`, no duplicates).
- Watch several streams at the same time in a responsive grid (one column on phones, more on wide screens).
- Per-stream controls: **Play / Pause** and **Remove**, plus a live status (Connecting, Live, Paused, Offline).
- Clear error messages with a **Try again** button.
- Automatic reconnection when the server is unreachable (for example, a free host waking up).
- The list of streams is remembered in the browser between visits.

## How it works

```
RTSP source ──RTSP──▶ Go backend + FFmpeg ──WebSocket (MPEG-TS)──▶ React app (JSMpeg canvas)
(camera or MediaMTX)      one FFmpeg per URL                          one tile per stream
```

1. The user adds an RTSP URL in the UI. Each tile opens a WebSocket to `GET /ws?url=<rtsp url>`.
2. The backend validates the URL and starts one **FFmpeg** process for it. FFmpeg reads the RTSP stream and re-encodes it to **MPEG-1 video in an MPEG-TS container**, written to standard output.
3. The backend reads FFmpeg's output and **broadcasts** it in binary WebSocket messages to every viewer of that URL.
4. In the browser, **JSMpeg** decodes the stream and draws it on a `<canvas>`.

Browsers cannot play RTSP directly, which is why FFmpeg is needed. MPEG-1 over WebSocket was chosen because it gives low latency and plays everywhere without plugins.

## Tech stack
| Part | Technology |
|---|---|
| Frontend | React 18, TypeScript, Vite, JSMpeg (`@cycjimmy/jsmpeg-player`) |
| Backend | Go 1.22, `gorilla/websocket`, FFmpeg (run as a child process) |
| Test streams | MediaMTX (RTSP server) + FFmpeg (publishes a test video) |
| Hosting | Vercel (frontend), Render with Docker (backend) |

## Project structure
```
RTSP-stream-Viewer/
├── README.md
├── backend/
│   ├── main.go            # server setup, routes, graceful shutdown
│   ├── config.go          # environment-variable configuration
│   ├── handler.go         # WebSocket endpoint, URL validation, health endpoint
│   ├── stream.go          # stream manager: FFmpeg lifecycle, fan-out to viewers
│   ├── Dockerfile         # builds the Go binary and installs FFmpeg (used by Render)
│   ├── docker-compose.yml # optional: MediaMTX for local testing (needs Docker)
│   └── scripts/
│       └── publish-test-stream.sh   # publishes a looping test video (macOS/Linux)
└── frontend/
    ├── index.html
    ├── package.json
    ├── .env.example
    └── src/
        ├── main.tsx           # app entry
        ├── App.tsx            # URL input, stream list, grid layout
        ├── StreamTile.tsx     # one player: status, play/pause, retry, errors
        ├── jsmpegSource.ts    # WebSocket source for JSMpeg (close codes, pause/play)
        ├── config.ts          # reads VITE_WS_URL
        └── styles.css
```

---

## Run locally

Docker is **not** required. You need four programs installed:

| Program | Used for | Check it works |
|---|---|---|
| Go 1.22+ | backend | `go version` |
| Node.js 18+ | frontend | `node -v` |
| FFmpeg | converting video | `ffmpeg -version` |
| [MediaMTX](https://github.com/bluenviron/mediamtx/releases) | local RTSP test server | download and unzip |

FFmpeg install: Windows `winget install Gyan.FFmpeg`, macOS `brew install ffmpeg`, Ubuntu/Debian `sudo apt install ffmpeg`. Open a new terminal afterwards so the command is found.

Run each step in its **own terminal** and leave all four open.

**1. Start the RTSP test server (MediaMTX)** from the folder where you unzipped it:
```powershell
.\mediamtx.exe        # macOS/Linux: ./mediamtx
```

**2. Publish a test video** to it (a moving test pattern with a running clock):
```bash
ffmpeg -re -f lavfi -i testsrc2=size=1280x720:rate=25 -c:v libx264 -preset ultrafast -tune zerolatency -pix_fmt yuv420p -g 50 -f rtsp -rtsp_transport tcp rtsp://127.0.0.1:8554/test
```
For a second stream, run the same command in another terminal with `smptebars` instead of `testsrc2` and `/cam2` at the end of the address.

**3. Start the backend:**
```bash
cd backend
go mod tidy
go run .              # listens on http://localhost:8080
```

**4. Start the frontend:**
```bash
cd frontend
npm install
cp .env.example .env  # Windows PowerShell: copy .env.example .env
npm run dev           # opens on http://localhost:5173
```

Open http://localhost:5173 and add `rtsp://127.0.0.1:8554/test`. The tile should switch from "Connecting" to "Live" within a couple of seconds.

*Optional (needs Docker):* `docker compose up -d` inside `backend/` starts MediaMTX instead of step 1.

---

## Configuration

### Backend environment variables
All are optional; the defaults work for local development.

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP port (hosting platforms set this automatically) |
| `ALLOWED_ORIGINS` | any | Comma-separated frontend origins allowed to connect, e.g. `https://rtsp-stream-viewer-nine.vercel.app` |
| `ALLOWED_HOSTS` | any | Restrict which RTSP hosts can be opened |
| `MAX_STREAMS` | `6` | Maximum number of simultaneous FFmpeg processes |
| `VIDEO_WIDTH` | `640` | Output width in pixels (height keeps the aspect ratio) |
| `VIDEO_FPS` | `25` | Output frame rate. MPEG-1 only allows 24, 25, 30, 50 or 60 |
| `VIDEO_BITRATE` | `600k` | Output video bitrate |
| `IDLE_GRACE_SECONDS` | `10` | How long FFmpeg keeps running after the last viewer leaves |
| `STALL_TIMEOUT_SECONDS` | `15` | FFmpeg is killed if no video data arrives for this long |
| `FFMPEG_PATH` | `ffmpeg` | Path to the FFmpeg program |

### Frontend environment variable
| Variable | Example | Description |
|---|---|---|
| `VITE_WS_URL` | `ws://localhost:8080/ws` (local) or `wss://rtsp-stream-viewer-0hg9.onrender.com/ws` (deployed) | WebSocket address of the backend. Use `wss://` when the site is served over HTTPS. The value is baked in at build time, so redeploy after changing it. |

---

## Backend API

| Endpoint | Description |
|---|---|
| `GET /ws?url=<rtsp-url>` | Upgrades to a WebSocket and streams the video. Binary messages are MPEG-TS data. |
| `GET /api/health` | Returns `{"streams":N,"clients":N,"maxStreams":N}` |
| `GET /` | Plain-text status message |

**Control messages** (client to server, text): `{"action":"pause"}` stops sending video to that client and `{"action":"play"}` resumes it.

**Errors** are reported through the WebSocket close code and reason:

| Close code | Meaning |
|---|---|
| `1008` | Invalid URL (the reason explains why) |
| `1011` | The stream could not be read, or FFmpeg kept failing |
| `1013` | Server is at capacity (`MAX_STREAMS`) |
| `1001` | Server shutting down or the stream stopped |

---

## Error handling
- **Invalid input:** the frontend checks the URL format first; the backend validates again (scheme, host, length, optional host allow-list).
- **Unreachable or wrong stream:** FFmpeg's error is captured and shown in the tile with a **Try again** button. A stream that has never worked fails fast (2 attempts) instead of retrying for a long time.
- **Stream drops while playing:** FFmpeg is restarted with an increasing delay, up to 5 attempts, then the viewers get a clear error.
- **Hung stream:** a watchdog kills FFmpeg if no data arrives within `STALL_TIMEOUT_SECONDS`.
- **Server unreachable from the browser:** the tile shows "Waiting for the server…" and retries up to 8 times (useful while a free host wakes up).
- **No leaked processes:** FFmpeg is stopped when the last viewer leaves, and all FFmpeg processes are terminated on server shutdown.
- **Credentials** inside an RTSP URL are masked in logs and error messages.

## Performance and scalability
- **One FFmpeg per stream, not per viewer.** Many viewers of the same URL share a single FFmpeg process.
- **Backpressure:** each viewer has a bounded queue; a slow viewer drops frames instead of slowing down everyone else.
- **Pause saves bandwidth:** pausing a tile also stops the server from sending that tile's video.
- **Resource cap:** `MAX_STREAMS` limits CPU use. The rest of the design is stateless per connection, so more capacity means running more instances (a possible next step would be a shared stream cache or sticky routing).
- **Tunable quality:** lower `VIDEO_WIDTH`, `VIDEO_FPS` or `VIDEO_BITRATE` to fit smaller servers.

---

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `ffmpeg not found` when the backend starts | FFmpeg is not on your PATH. Install it and open a new terminal, or set `FFMPEG_PATH`. |
| Tile shows an error with `404 Not Found` | Nothing is publishing to that path. Start the test video (step 2) and check the stream name. |
| Error mentioning `-138` / timeout | FFmpeg could not reach the RTSP server. Make sure MediaMTX is running and the address is correct. |
| Error `0xffffffea` / "Nothing was written into output file" | An unsupported frame rate was set. Use `VIDEO_FPS` 24, 25, 30, 50 or 60. |
| Tile stuck on "Waiting for the server…" | The free backend is waking up. Wait about a minute; it retries automatically. |
| PowerShell error about the `&` character | Wrap RTSP URLs containing `&` in double quotes when using them in the terminal. |
| Choppy video with several streams | Lower `VIDEO_WIDTH` (e.g. `480` or `320`) and `MAX_STREAMS`, or use a larger server. |

## Known limitations and next steps
- Video is re-encoded to MPEG-1, which gives low latency but modest picture quality and uses server CPU. Alternatives such as fragmented MP4 over Media Source Extensions would give better quality.
- Audio is not forwarded.
- Free hosting has little CPU and sleeps when idle; a paid instance is recommended for many simultaneous streams.
- Possible improvements: authentication, a persistent stream list on the server, automated tests, and horizontal scaling with a shared stream cache.
