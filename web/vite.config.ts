import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";
import os from "os";

/// <reference types="vitest" />

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const apiProxyTarget = env.VITE_API_PROXY_TARGET || "http://localhost:8090";
  const hmrClientPort = Number(env.VITE_HMR_CLIENT_PORT || "");
  // Vite only lets bare IPv4 hosts through unlisted, so a dev server reached by
  // name — the local mDNS name, or a Tailscale MagicDNS name when someone views
  // the dev UI from another device on the tailnet — has to be allowed here.
  // VITE_ALLOWED_HOSTS adds any others (comma-separated).
  const allowedHosts = [
    "prairie.local",
    ".ts.net",
    os.hostname(),
    ...(env.VITE_ALLOWED_HOSTS || "")
      .split(",")
      .map((host) => host.trim())
      .filter(Boolean),
  ];
  // Remote backends (e.g. the hosted dev server) sit behind vhost-routing
  // proxies that reject a localhost Host header; local backends don't care
  // either way but keeping Host intact preserves existing behavior.
  const apiProxyIsLocal = /^https?:\/\/(localhost|127\.0\.0\.1|\[?::1\]?)(:|\/|$)/.test(
    apiProxyTarget,
  );

  return {
    plugins: [react(), tailwindcss()],
    worker: {
      format: "es",
    },
    optimizeDeps: {
      // jassub spawns its own module worker with import.meta.url paths; the
      // dep optimizer rewrites those into .vite/deps where the worker file
      // doesn't exist, so the ASS renderer never initializes in dev.
      exclude: ["jassub"],
      // CJS deps of the excluded package still need prebundling for ESM interop.
      include: ["jassub > throughput", "jassub > rvfc-polyfill"],
    },
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
        "@pdfjs": path.resolve(__dirname, "./public/vendor/pdfjs"),
      },
    },
    server: {
      host: "0.0.0.0",
      allowedHosts,
      hmr:
        Number.isFinite(hmrClientPort) && hmrClientPort > 0
          ? { clientPort: hmrClientPort }
          : undefined,
      proxy: {
        "/api": {
          target: apiProxyTarget,
          changeOrigin: !apiProxyIsLocal,
          secure: true,
          ws: true,
        },
      },
    },
    test: {
      environment: "jsdom",
      globals: true,
      setupFiles: ["./src/test-setup.ts"],
      // Enforce 90% on pure helpers under src/lib/. Prefer modules that already
      // have *.test.ts coverage; write tests before adding a file here.
      // Thin React Query / mutation wrappers are left out until they are
      // meaningfully unit-tested.
      coverage: {
        provider: "v8",
        include: [
          "src/lib/artworkUrl.ts",
          "src/lib/liveTVGuide.ts",
          "src/lib/datetime.ts",
          "src/lib/filterEasyMode.ts",
          "src/lib/jellyfinCompat.ts",
          "src/lib/impersonationSession.ts",
          "src/lib/chunkedUpload.ts",
          "src/lib/calendarWeek.ts",
          "src/lib/autoscanLabels.ts",
          "src/lib/documentTitle.ts",
          "src/lib/mangaChapters.ts",
          "src/lib/collectionDisplayFilters.ts",
          "src/lib/carouselEmbla.ts",
          "src/lib/mediaFormat.ts",
          "src/lib/pluginRouteHref.ts",
          "src/lib/recipes.ts",
          "src/lib/recommendation-provider-presets.ts",
          "src/lib/videoRange.ts",
          "src/lib/webhookSync.ts",
          "src/lib/queryInvalidation.ts",
          "src/utils/storage.ts",
        ],
        thresholds: {
          statements: 90,
          lines: 90,
          functions: 90,
          branches: 90,
        },
      },
    },
  };
});
