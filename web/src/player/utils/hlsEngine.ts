export type HLSEngineV3 = "native" | "hlsjs" | "unsupported";

interface HLSJSSupportV3 {
  isSupported(): boolean;
}

export type ResolvedHLSEngineV3<T extends HLSJSSupportV3> =
  | { engine: "native" | "unsupported" }
  | { engine: "hlsjs"; hlsjs: T };

export function isSafariBrowserV3(userAgent: string): boolean {
  return (
    /Safari\//i.test(userAgent) &&
    !/(?:Chrome|Chromium|CriOS|Edg|EdgiOS|OPR|OPiOS|Firefox|FxiOS)\//i.test(userAgent)
  );
}

/**
 * Predicts, before a plan exists, whether the video player will play an HLS
 * plan through hls.js rather than the media element. It mirrors the player's
 * engine choice — Safari always stays native — and hls.js's own baseline, a
 * Media Source implementation, without loading hls.js itself.
 */
export function hlsJSEngineExpectedV3(userAgent: string, mediaSourceAvailable: boolean): boolean {
  return mediaSourceAvailable && !isSafariBrowserV3(userAgent);
}

/** {@link hlsJSEngineExpectedV3} for the running browser. */
export function currentHLSJSEngineExpectedV3(): boolean {
  if (typeof navigator === "undefined") return false;
  const scope = globalThis as { MediaSource?: unknown; ManagedMediaSource?: unknown };
  return hlsJSEngineExpectedV3(
    navigator.userAgent,
    typeof scope.MediaSource === "function" || typeof scope.ManagedMediaSource === "function",
  );
}

function nativeHLSPreferred(nativeSupported: boolean, preferNativeHLS: boolean): boolean {
  return preferNativeHLS && nativeSupported;
}

export function selectHLSEngineV3(
  _dynamicRange: string | undefined,
  nativeSupported: boolean,
  hlsJSSupported: boolean,
  preferNativeHLS = false,
): HLSEngineV3 {
  if (nativeHLSPreferred(nativeSupported, preferNativeHLS)) return "native";
  if (hlsJSSupported) return "hlsjs";
  if (nativeSupported) return "native";
  return "unsupported";
}

export async function resolveHLSEngineV3<T extends HLSJSSupportV3>(
  dynamicRange: string | undefined,
  nativeSupported: boolean,
  loadHLSJS: () => Promise<T>,
  onHLSJSUnavailable?: (error: unknown) => void,
  preferNativeHLS = false,
): Promise<ResolvedHLSEngineV3<T>> {
  if (nativeHLSPreferred(nativeSupported, preferNativeHLS)) {
    return { engine: "native" };
  }

  try {
    const hlsjs = await loadHLSJS();
    const engine = selectHLSEngineV3(
      dynamicRange,
      nativeSupported,
      hlsjs.isSupported(),
      preferNativeHLS,
    );
    return engine === "hlsjs" ? { engine, hlsjs } : { engine };
  } catch (error) {
    if (nativeSupported) {
      // Falling back to the media element keeps playback alive, but the
      // preferred engine's loss (and its tuned retry/recovery behavior) must
      // not be silent — hand the cause to the caller before degrading.
      onHLSJSUnavailable?.(error);
      return { engine: "native" };
    }
    throw error;
  }
}
