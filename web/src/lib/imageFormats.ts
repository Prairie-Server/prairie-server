/**
 * One-time raster decode capability detection for Prairie web clients.
 * Results are cached in localStorage and advertised to the server via
 * X-Prairie-Image-Formats on API requests.
 *
 * Defaults optimistically include AVIF so ArtworkImage prefers AVIF siblings
 * on first paint; unsupported clients fall through via onError. Detection can
 * only remove formats, never leave AVIF permanently off after a bad probe.
 */

export type RasterFormat = "avif" | "webp" | "png";

/** Bump when probe logic changes so stale localStorage results are ignored. */
const STORAGE_KEY = "prairie.imageFormats.v2";
const LEGACY_STORAGE_KEYS = ["prairie.imageFormats"] as const;

/** Optimistic preference until (and unless) probing proves otherwise. */
const DEFAULT_FORMATS: RasterFormat[] = ["avif", "webp", "png"];

let cached: RasterFormat[] | null = null;
let detectPromise: Promise<RasterFormat[]> | null = null;
const listeners = new Set<() => void>();

function notifyListeners() {
  for (const listener of listeners) {
    listener();
  }
}

function parseStored(value: string | null): RasterFormat[] | null {
  if (!value?.trim()) return null;
  const out: RasterFormat[] = [];
  const seen = new Set<RasterFormat>();
  for (const part of value.split(",")) {
    const token = part.trim().toLowerCase();
    if (token !== "avif" && token !== "webp" && token !== "png") continue;
    const format = token as RasterFormat;
    if (seen.has(format)) continue;
    seen.add(format);
    out.push(format);
  }
  return out.length > 0 ? out : null;
}

function readStoredFormats(): RasterFormat[] | null {
  if (typeof localStorage === "undefined") return null;
  const current = parseStored(localStorage.getItem(STORAGE_KEY));
  if (current) return current;
  // Ignore legacy keys — v1 often stored "webp,png" after a failed AVIF stub probe.
  for (const key of LEGACY_STORAGE_KEYS) {
    localStorage.removeItem(key);
  }
  return null;
}

type ImageDecoderCtor = {
  isTypeSupported?: (type: string) => boolean | Promise<boolean>;
};

async function isTypeSupported(mime: string): Promise<boolean | null> {
  const ctor = (globalThis as { ImageDecoder?: ImageDecoderCtor }).ImageDecoder;
  if (!ctor?.isTypeSupported) return null;
  try {
    return await Promise.resolve(ctor.isTypeSupported(mime));
  } catch {
    return null;
  }
}

async function canDecodeMime(mime: string, bytes: Uint8Array): Promise<boolean> {
  if (typeof createImageBitmap !== "function") {
    return false;
  }
  try {
    const copy = new Uint8Array(bytes.byteLength);
    copy.set(bytes);
    const blob = new Blob([copy], { type: mime });
    const bitmap = await createImageBitmap(blob);
    bitmap.close();
    return true;
  } catch {
    return false;
  }
}

// Real 32×32 favicon AVIF from web/public/favicon-32.avif — incomplete ftyp-only
// stubs fail createImageBitmap even on AVIF-capable browsers.
const AVIF_PROBE_BYTES = Uint8Array.from(
  atob(
    "AAAAIGZ0eXBhdmlmAAAAAGF2aWZtaWYxbWlhZk1BMUEAAAD5bWV0YQAAAAAAAAAvaGRscgAAAAAAAAAAcGljdAAAAAAAAAAAAAAAAFBpY3R1cmVIYW5kbGVyAAAAAA5waXRtAAAAAAABAAAAHmlsb2MAAAAARAAAAQABAAAAAQAAASEAAAFQAAAAKGlpbmYAAAAAAAEAAAAaaW5mZQIAAAAAAQAAYXYwMUNvbG9yAAAAAGppcHJwAAAAS2lwY28AAAAUaXNwZQAAAAAAAAAgAAAAIAAAABBwaXhpAAAAAAMICAgAAAAMYXYxQ4EgAAAAAAATY29scm5jbHgAAgACAACAAAAAF2lwbWEAAAAAAAAAAQABBAECgwQAAAFYbWRhdAoIOBE/9tAQ0AIywwIUgAkYssSEiATQtOzrxpnjvxj0P2Yrewu/43i6oBYe50SNFLolwBqak7yBssJJnlaX1t4sAPtHLKsNDuk27cKN/vO8kR55hfGmodJ0CLFghIjJ9EqaTDiCU0bu7+ADOb2Gb135bofNXMgrq+lMKYC4Z9lcQh9FRKeqz9I7AF9aiSu5MkTavxJfHYiA12cpC+mKj/i4hMGIDRIfh+PgaA+wwvbrDGkF9A/iwoL4+gt2uR1vqw1ksl6gyvPA2zLErSWIeysXVfl2yJrv8X7hbu5dTbDR/3TtUOnpH+X7pZtsFsP2bw8vh74xNoLhxlvbvLdQbux7Jua7/wH2/1nk8qhhbBImKxwG07ooNY1mzKSmeLgfCe2B7vhEBbuZaRGwDz8BAmyrJ7mYGoueBAXIVZnJsRkxRuwwG9Q6qRZz8lUL1Bi4IA==",
  ),
  (c) => c.charCodeAt(0),
);

const WEBP_PROBE_BYTES = Uint8Array.from([
  0x52, 0x49, 0x46, 0x46, 0x24, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50, 0x56, 0x50, 0x38, 0x20,
  0x18, 0x00, 0x00, 0x00, 0x30, 0x01, 0x00, 0x9d, 0x01, 0x2a, 0x01, 0x00, 0x01, 0x00, 0x02, 0x00,
  0x34, 0x25, 0xa4, 0x00, 0x03, 0x70, 0x00, 0xfe, 0xfb, 0xfd, 0x50, 0x00,
]);

async function supportsFormat(mime: string, bytes: Uint8Array): Promise<boolean> {
  const typed = await isTypeSupported(mime);
  if (typed === true) return true;
  if (typed === false) return false;
  return canDecodeMime(mime, bytes);
}

async function probeFormats(): Promise<RasterFormat[]> {
  const out: RasterFormat[] = ["png"];
  if (await supportsFormat("image/webp", WEBP_PROBE_BYTES)) {
    out.unshift("webp");
  }
  if (await supportsFormat("image/avif", AVIF_PROBE_BYTES)) {
    out.unshift("avif");
  }
  return out;
}

function setCachedFormats(formats: RasterFormat[]): RasterFormat[] {
  cached = formats;
  if (typeof localStorage !== "undefined") {
    localStorage.setItem(STORAGE_KEY, formats.join(","));
    for (const key of LEGACY_STORAGE_KEYS) {
      localStorage.removeItem(key);
    }
  }
  notifyListeners();
  return formats;
}

/** Current best-known raster preference list (sync; optimistic until probing finishes). */
export function getImageFormats(): RasterFormat[] {
  if (cached) return cached;
  if (typeof localStorage !== "undefined") {
    const stored = readStoredFormats();
    if (stored) {
      cached = stored;
      return stored;
    }
  }
  return DEFAULT_FORMATS;
}

/** Subscribe to format-list changes (detection complete / cache reset). */
export function subscribeImageFormats(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Probe decode support once, persist, and return the ordered preference list. */
export async function detectImageFormats(): Promise<RasterFormat[]> {
  if (cached) return cached;
  if (detectPromise) return detectPromise;
  detectPromise = (async () => {
    const stored = readStoredFormats();
    if (stored) {
      return setCachedFormats(stored);
    }
    const detected = await probeFormats();
    return setCachedFormats(detected);
  })();
  try {
    return await detectPromise;
  } finally {
    detectPromise = null;
  }
}

/** Value for X-Prairie-Image-Formats request header. */
export function imageFormatsHeaderValue(): string {
  return getImageFormats().join(",");
}

/** Reorder raster URLs by the client's supported format preference. */
export function orderRasterCandidates(
  byFormat: Partial<Record<RasterFormat, string | null | undefined>>,
  preferred: readonly RasterFormat[] = getImageFormats(),
): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const format of preferred) {
    const url = byFormat[format]?.trim();
    if (!url || seen.has(url)) continue;
    seen.add(url);
    out.push(url);
  }
  return out;
}

/** @internal Test helper — clears in-memory and persisted format cache. */
export function resetImageFormatsCacheForTests(): void {
  cached = null;
  detectPromise = null;
  if (typeof localStorage !== "undefined") {
    localStorage.removeItem(STORAGE_KEY);
    for (const key of LEGACY_STORAGE_KEYS) {
      localStorage.removeItem(key);
    }
  }
  notifyListeners();
}
