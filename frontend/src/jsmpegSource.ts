/**
 * Custom JSMpeg source that wraps a WebSocket. Unlike the built-in one it
 * exposes the close code/reason (for error messages) and can send
 * pause/play commands to the backend.
 */
export interface SourceOptions {
  onSocketClose?: (code: number, reason: string) => void;
}

export class ControlledWebSocketSource {
  established = false;
  completed = false;
  progress = 0;
  private ws: WebSocket | null = null;
  private destination: { write(data: ArrayBuffer): void } | null = null;

  constructor(private url: string, private options: SourceOptions) {}

  connect(destination: { write(data: ArrayBuffer): void }) {
    this.destination = destination;
    this.start();
  }

  start() {
    if (this.ws) return; // idempotent
    const ws = new WebSocket(this.url);
    ws.binaryType = "arraybuffer";
    ws.onopen = () => {
      this.established = true;
      this.progress = 1;
    };
    ws.onmessage = (e) => {
      if (e.data instanceof ArrayBuffer) this.destination?.write(e.data);
    };
    ws.onclose = (e) => {
      this.completed = true;
      this.options.onSocketClose?.(e.code, e.reason);
    };
    this.ws = ws;
  }

  resume() {}

  destroy() {
    if (!this.ws) return;
    this.ws.onclose = null; // don't report our own shutdown as an error
    this.ws.close();
    this.ws = null;
  }

  send(action: "pause" | "play") {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ action }));
    }
  }
}
