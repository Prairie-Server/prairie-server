import { createRoot } from "react-dom/client";
import App from "./App";
import { detectImageFormats } from "./lib/imageFormats";
import { installPreloadErrorReload } from "./lib/reloadOnPreloadError";
import { initPointerCapability } from "./lib/pointerCapability";
import { ensureStorageSchema } from "./utils/storage";
import "./fonts.css";
import "./app.css";

// Bump/record storage schema without clearing auth/session keys across upgrades.
ensureStorageSchema();
installPreloadErrorReload();
void detectImageFormats();
// Before render, so the first paint already knows whether hover reveals apply.
initPointerCapability();

const root = document.getElementById("root");
if (root === null) throw new Error("Root element #root not found");
createRoot(root).render(<App />);
