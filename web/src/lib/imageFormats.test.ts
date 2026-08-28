import { afterEach, describe, expect, it, vi } from "vitest";
import {
  detectImageFormats,
  getImageFormats,
  imageFormatsHeaderValue,
  orderRasterCandidates,
  resetImageFormatsCacheForTests,
  subscribeImageFormats,
} from "./imageFormats";

describe("imageFormats", () => {
  afterEach(() => {
    resetImageFormatsCacheForTests();
    vi.unstubAllGlobals();
  });

  it("defaults optimistically to avif before probing", () => {
    expect(getImageFormats()).toEqual(["avif", "webp", "png"]);
    expect(imageFormatsHeaderValue()).toBe("avif,webp,png");
  });

  it("orders raster candidates by detected preference", () => {
    localStorage.setItem("prairie.imageFormats.v2", "webp,png");
    expect(
      orderRasterCandidates({
        avif: "https://cdn/poster.avif",
        webp: "https://cdn/poster.webp",
        png: "https://cdn/poster.png",
      }),
    ).toEqual(["https://cdn/poster.webp", "https://cdn/poster.png"]);
  });

  it("ignores legacy v1 storage that omitted avif after a failed stub probe", () => {
    localStorage.setItem("prairie.imageFormats", "webp,png");
    expect(getImageFormats()).toEqual(["avif", "webp", "png"]);
    expect(localStorage.getItem("prairie.imageFormats")).toBeNull();
  });

  it("reads cached formats for the request header", () => {
    localStorage.setItem("prairie.imageFormats.v2", "avif,webp,png");
    expect(imageFormatsHeaderValue()).toBe("avif,webp,png");
    expect(getImageFormats()).toEqual(["avif", "webp", "png"]);
  });

  it("detects and persists formats", async () => {
    const formats = await detectImageFormats();
    expect(formats.length).toBeGreaterThan(0);
    expect(formats).toContain("png");
    expect(localStorage.getItem("prairie.imageFormats.v2")).toBe(
      formats.join(","),
    );
  });

  it("uses ImageDecoder.isTypeSupported when available", async () => {
    vi.stubGlobal("ImageDecoder", {
      isTypeSupported: (type: string) =>
        type === "image/avif" || type === "image/webp",
    });
    const formats = await detectImageFormats();
    expect(formats[0]).toBe("avif");
    expect(formats).toContain("webp");
  });

  it("notifies subscribers when formats settle", async () => {
    const seen: string[] = [];
    const unsubscribe = subscribeImageFormats(() => {
      seen.push(getImageFormats().join(","));
    });
    await detectImageFormats();
    unsubscribe();
    expect(seen.length).toBeGreaterThan(0);
  });
});
