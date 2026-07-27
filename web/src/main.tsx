import { createRoot } from "react-dom/client";
import App from "./App";
import { installPreloadErrorReload } from "./lib/reloadOnPreloadError";
import { ensureStorageSchema } from "./utils/storage";
import "./app.css";

// Bump/record storage schema without clearing auth/session keys across upgrades.
ensureStorageSchema();
installPreloadErrorReload();

// Activate deferred alternate-theme fonts without an inline onload handler (CSP).
const themeFonts = document.getElementById("prairie-theme-fonts");
if (themeFonts instanceof HTMLLinkElement) {
  const activate = () => {
    themeFonts.media = "all";
  };
  if (themeFonts.sheet) {
    activate();
  } else {
    themeFonts.addEventListener("load", activate, { once: true });
  }
}

const root = document.getElementById("root");
if (root === null) throw new Error("Root element #root not found");
createRoot(root).render(<App />);
