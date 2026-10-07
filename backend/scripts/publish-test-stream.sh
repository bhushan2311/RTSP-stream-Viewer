#!/usr/bin/env bash
# Publishes a looping synthetic test stream to a local MediaMTX server.
# Usage: ./scripts/publish-test-stream.sh [name] [source]
#   name   : stream path (default: test)  -> rtsp://localhost:8554/<name>
#   source : testsrc2 | smptebars | mandelbrot (default: testsrc2)
set -euo pipefail
NAME="${1:-test}"
SOURCE="${2:-testsrc2}"
exec ffmpeg -re -f lavfi -i "${SOURCE}=size=1280x720:rate=25" \
  -c:v libx264 -preset ultrafast -tune zerolatency -pix_fmt yuv420p -g 50 \
  -f rtsp -rtsp_transport tcp "rtsp://localhost:8554/${NAME}"
