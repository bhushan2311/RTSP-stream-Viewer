import { createRoot } from "react-dom/client";
import App from "./App";
import "./styles.css";

// StrictMode is intentionally not used: it mounts effects twice in development,
// which would open and tear down each video connection twice.
createRoot(document.getElementById("root")!).render(<App />);
