import { createRoot } from "react-dom/client";
import App from "./App";
import { installPreloadErrorReload } from "./lib/reloadOnPreloadError";
import { ensureStorageSchema } from "./utils/storage";
import "./app.css";

// Bump/record storage schema without clearing auth/session keys across upgrades.
ensureStorageSchema();
installPreloadErrorReload();

// Activate deferred alternate-theme fonts without an inline onload handler (CSP).
// media="print" kept the link non-blocking during HTML parse; flip once the
// module runs. Avoid link.sheet / load listeners — cross-origin Google Fonts
// often expose a null sheet and can miss a load that already fired.
const themeFonts = document.getElementById("prairie-theme-fonts");
if (themeFonts instanceof HTMLLinkElement) {
  themeFonts.media = "all";
}

const root = document.getElementById("root");
if (root === null) throw new Error("Root element #root not found");
createRoot(root).render(<App />);
