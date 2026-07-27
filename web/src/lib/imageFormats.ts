/**
 * One-time raster decode capability detection for Prairie web clients.
 * Results are cached in localStorage and advertised to the server via
 * X-Prairie-Image-Formats on API requests.
 */

export type RasterFormat = "avif" | "webp" | "png";

const STORAGE_KEY = "prairie.imageFormats";
const DEFAULT_FORMATS: RasterFormat[] = ["webp", "png"];

let cached: RasterFormat[] | null = null;
let detectPromise: Promise<RasterFormat[]> | null = null;

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

async function canDecodeMime(mime: string): Promise<boolean> {
  if (typeof createImageBitmap !== "function") {
    return false;
  }
  const bytes =
    mime === "image/avif"
      ? new Uint8Array([
          0x00, 0x00, 0x00, 0x1c, 0x66, 0x74, 0x79, 0x70, 0x61, 0x76, 0x69, 0x66, 0x00, 0x00, 0x00,
          0x00, 0x61, 0x76, 0x69, 0x66, 0x6d, 0x69, 0x66, 0x31, 0x6d, 0x69, 0x61, 0x66, 0x00, 0x00,
          0x00, 0x00,
        ])
      : new Uint8Array([
          0x52, 0x49, 0x46, 0x46, 0x24, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50, 0x56, 0x50, 0x38,
          0x20, 0x18, 0x00, 0x00, 0x00, 0x30, 0x01, 0x00, 0x9d, 0x01, 0x2a, 0x01, 0x00, 0x01, 0x00,
          0x02, 0x00, 0x34, 0x25, 0xa4, 0x00, 0x03, 0x70, 0x00, 0xfe, 0xfb, 0xfd, 0x50, 0x00,
        ]);
  try {
    const blob = new Blob([bytes], { type: mime });
    const bitmap = await createImageBitmap(blob);
    bitmap.close();
    return true;
  } catch {
    return false;
  }
}

async function probeFormats(): Promise<RasterFormat[]> {
  const out: RasterFormat[] = ["png"];
  if (await canDecodeMime("image/webp")) {
    out.unshift("webp");
  }
  if (await canDecodeMime("image/avif")) {
    out.unshift("avif");
  }
  return out;
}

/** Current best-known raster preference list (sync; may be conservative until detect finishes). */
export function getImageFormats(): RasterFormat[] {
  if (cached) return cached;
  if (typeof localStorage !== "undefined") {
    const stored = parseStored(localStorage.getItem(STORAGE_KEY));
    if (stored) {
      cached = stored;
      return stored;
    }
  }
  return DEFAULT_FORMATS;
}

/** Probe decode support once, persist, and return the ordered preference list. */
export async function detectImageFormats(): Promise<RasterFormat[]> {
  if (cached) return cached;
  if (detectPromise) return detectPromise;
  detectPromise = (async () => {
    const stored =
      typeof localStorage !== "undefined" ? parseStored(localStorage.getItem(STORAGE_KEY)) : null;
    if (stored) {
      cached = stored;
      return stored;
    }
    const detected = await probeFormats();
    cached = detected;
    if (typeof localStorage !== "undefined") {
      localStorage.setItem(STORAGE_KEY, detected.join(","));
    }
    return detected;
  })();
  return detectPromise;
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
  }
}
