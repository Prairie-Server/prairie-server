import { readFileSync, writeFileSync } from "node:fs";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";
import { defineConfig, loadEnv, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";
import os from "os";

/// <reference types="vitest" />

const PRECOMPRESS_MIN_BYTES = 1024;

function precompressStaticAssets(): Plugin {
  return {
    name: "precompress-static-assets",
    apply: "build",
    writeBundle(options, bundle) {
      if (!options.dir) return;

      for (const output of Object.values(bundle)) {
        if (!/\.(?:css|js)$/.test(output.fileName)) continue;

        // Read the written file rather than the generateBundle value: later
        // Rollup hooks can still finalize chunk bytes before they reach disk.
        const filePath = path.resolve(options.dir, output.fileName);
        const bytes = readFileSync(filePath);
        if (bytes.byteLength < PRECOMPRESS_MIN_BYTES) continue;

        writeFileSync(
          `${filePath}.br`,
          brotliCompressSync(bytes, {
            params: { [constants.BROTLI_PARAM_QUALITY]: 11 },
          }),
        );
        writeFileSync(`${filePath}.gz`, gzipSync(bytes, { level: 9 }));
      }
    },
  };
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const apiProxyTarget = env.VITE_API_PROXY_TARGET || "http://localhost:8090";
  const hmrClientPort = Number(env.VITE_HMR_CLIENT_PORT || "");
  // Vite only lets bare IPv4 hosts through unlisted, so a dev server reached by
  // name — the local mDNS name, or a Tailscale MagicDNS name when someone views
  // the dev UI from another device on the tailnet — has to be allowed here.
  // VITE_ALLOWED_HOSTS adds any others (comma-separated).
  const allowedHosts = [
    "silo.local",
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
    plugins: [react(), tailwindcss(), precompressStaticAssets()],
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
      environment: "happy-dom",
      globals: true,
      setupFiles: ["./src/test-setup.ts"],
      pool: "forks",
      fileParallelism: true,
      maxWorkers: "50%",
      // Enforce 95% on pure helpers under src/lib/. Prefer modules that already
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
        // Per-shard runs only see a slice of tests; enforce thresholds on the
        // merged report (CI sets VITEST_SHARD=1). Local/full runs keep the gate.
        thresholds: process.env.VITEST_SHARD
          ? undefined
          : {
              statements: 95,
              lines: 95,
              functions: 95,
              branches: 95,
            },
      },
    },
  };
});
