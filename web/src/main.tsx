import { createRoot } from "react-dom/client";
import App from "./App";
import { installPreloadErrorReload } from "./lib/reloadOnPreloadError";
import { ensureStorageSchema } from "./utils/storage";
import "./app.css";

// Bump/record storage schema without clearing auth/session keys across upgrades.
ensureStorageSchema();
installPreloadErrorReload();

// Alternate-theme fonts stay media="print" until a non-default theme is
// selected (see ThemeProvider → ensureThemeFontsLoaded). That keeps ~160KB of
// Outfit/Manrope/Urbanist off the default Prairie Dusk first paint.

const root = document.getElementById("root");
if (root === null) throw new Error("Root element #root not found");
createRoot(root).render(<App />);
