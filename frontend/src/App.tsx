import { FormEvent, useEffect, useState } from "react";
import { StreamTile } from "./StreamTile";

interface Item {
  id: string;
  url: string;
}

const STORAGE_KEY = "rtsp-viewer.streams";

function loadItems(): Item[] {
  try {
    const parsed = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "[]");
    return Array.isArray(parsed) ? parsed.filter((i) => i?.id && i?.url) : [];
  } catch {
    return [];
  }
}

/** Returns an error message, or "" if the address is acceptable. */
function validate(raw: string): string {
  if (!raw) return "Enter an RTSP address.";
  try {
    const u = new URL(raw);
    if (u.protocol !== "rtsp:" && u.protocol !== "rtsps:") {
      return "The address must start with rtsp:// or rtsps://";
    }
    if (!u.hostname) return "The address needs a host name.";
  } catch {
    return "That doesn't look like a valid address.";
  }
  return "";
}

/** Short display name without credentials. */
function labelFor(raw: string): string {
  try {
    const u = new URL(raw);
    return (u.host + u.pathname).replace(/\/$/, "");
  } catch {
    return raw;
  }
}

export default function App() {
  const [items, setItems] = useState<Item[]>(loadItems);
  const [input, setInput] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(items));
    } catch {
      /* storage unavailable: the list just won't persist */
    }
  }, [items]);

  const addStream = (e: FormEvent) => {
    e.preventDefault();
    const url = input.trim();
    const problem = validate(url);
    if (problem) return setError(problem);
    if (items.some((i) => i.url === url)) return setError("That stream is already on the wall.");
    setItems((prev) => [...prev, { id: crypto.randomUUID(), url }]);
    setInput("");
    setError("");
  };

  return (
    <>
      <div className="top">
        <h1>RTSP stream viewer</h1>
        <form className="add" onSubmit={addStream} noValidate>
          <input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="rtsp://host:8554/stream"
            aria-label="RTSP stream address"
            aria-invalid={!!error}
            aria-describedby="add-error"
            spellCheck={false}
            autoComplete="off"
          />
          <button className="btn btn-primary" type="submit">Add stream</button>
          <p id="add-error" className="field-error" role="alert">{error}</p>
        </form>
      </div>

      <main className="wall">
        {items.length === 0 ? (
          <div className="empty">
            <h2>No streams yet</h2>
            <p>Paste an RTSP address above. It starts playing as soon as you add it, and you can add as many as the server allows.</p>
          </div>
        ) : (
          items.map((item) => (
            <StreamTile
              key={item.id}
              url={item.url}
              label={labelFor(item.url)}
              onRemove={() => setItems((prev) => prev.filter((i) => i.id !== item.id))}
            />
          ))
        )}
      </main>
    </>
  );
}
