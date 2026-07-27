import { createRoot } from "react-dom/client";
import App from "./App";
import { installPreloadErrorReload } from "./lib/reloadOnPreloadError";
import { ensureStorageSchema } from "./utils/storage";
import "./styles/fonts-default.css";
import "./app.css";

// Bump/record storage schema without clearing auth/session keys across upgrades.
ensureStorageSchema();
installPreloadErrorReload();

// Alternate-theme fonts (Outfit/Manrope/Urbanist) load on demand via
// ThemeProvider → ensureThemeFontsLoaded — not on the prairie-dusk critical path.

const root = document.getElementById("root");
if (root === null) throw new Error("Root element #root not found");
createRoot(root).render(<App />);
