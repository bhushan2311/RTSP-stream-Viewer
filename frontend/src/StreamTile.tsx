import { useEffect, useRef, useState } from "react";
import JSMpeg from "@cycjimmy/jsmpeg-player";
import { ControlledWebSocketSource } from "./jsmpegSource";
import { WS_URL } from "./config";

type Status = "connecting" | "live" | "paused" | "error";

const STATUS_LABEL: Record<Status, string> = {
  connecting: "Connecting",
  live: "Live",
  paused: "Paused",
  error: "Offline",
};
const MAX_AUTO_RETRIES = 8;
const RETRY_DELAY_MS = 4000;

function describeClose(code: number, reason: string): string {
  switch (code) {
    case 1008:
      return reason || "This address is not valid.";
    case 1011:
      return reason || "The stream could not be read.";
    case 1013:
      return "The server is at capacity. Remove a stream or try again shortly.";
    default:
      return "Lost the connection to the server.";
  }
}

interface Props {
  url: string;
  label: string;
  onRemove: () => void;
}

export function StreamTile({ url, label, onRemove }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const playerRef = useRef<any>(null);
  const retries = useRef(0);
  const [status, setStatus] = useState<Status>("connecting");
  const [message, setMessage] = useState("");
  const [attempt, setAttempt] = useState(0); // bump to reconnect

  useEffect(() => {
    let timer: number | undefined;
    let gotFrame = false;
    setStatus("connecting");
    setMessage("");

    const player = new JSMpeg.Player(`${WS_URL}?url=${encodeURIComponent(url)}`, {
      canvas: canvasRef.current,
      source: ControlledWebSocketSource,
      audio: false,
      autoplay: true,
      onVideoDecode: () => {
        if (gotFrame) return;
        gotFrame = true;
        retries.current = 0;
        setStatus("live");
        setMessage("");
      },
      onSocketClose: (code: number, reason: string) => {
        if (code === 1000) return;
        // Server unreachable or restarting (e.g. free hosting waking up): retry quietly.
        if ((code === 1006 || code === 1001) && retries.current < MAX_AUTO_RETRIES) {
          retries.current += 1;
          setStatus("connecting");
          setMessage("Waiting for the server…");
          timer = window.setTimeout(() => setAttempt((a) => a + 1), RETRY_DELAY_MS);
          return;
        }
        setStatus("error");
        setMessage(describeClose(code, reason));
      },
    });
    playerRef.current = player;

    return () => {
      window.clearTimeout(timer);
      player.destroy();
      playerRef.current = null;
    };
  }, [url, attempt]);

  const togglePause = () => {
    const player = playerRef.current;
    if (!player) return;
    if (status === "live") {
      player.source.send("pause"); // stop the server sending video to this tile
      player.pause();
      setStatus("paused");
    } else if (status === "paused") {
      player.source.send("play");
      player.play();
      setStatus("live");
    }
  };

  const retry = () => {
    retries.current = 0;
    setAttempt((a) => a + 1);
  };

  const canPause = status === "live" || status === "paused";

  return (
    <article className={`tile tile-${status}`}>
      <header className="tile-bar">
        <span className="dot" aria-hidden="true" />
        <span className="tile-name" title={label}>{label}</span>
        <span className="tile-status">{STATUS_LABEL[status]}</span>
      </header>

      <div className="screen">
        {/* key forces a fresh canvas on every reconnect (WebGL contexts can't be reused) */}
        <canvas key={attempt} ref={canvasRef} />
        {status !== "live" && status !== "paused" && (
          <div className="overlay" role="status">
            <p>{message || (status === "connecting" ? "Connecting to stream…" : "")}</p>
            {status === "error" && (
              <button className="btn" onClick={retry}>Try again</button>
            )}
          </div>
        )}
      </div>

      <footer className="tile-actions">
        <button className="btn" onClick={togglePause} disabled={!canPause}>
          {status === "paused" ? "Play" : "Pause"}
        </button>
        <button className="btn btn-quiet" onClick={onRemove}>Remove</button>
      </footer>
    </article>
  );
}
