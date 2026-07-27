import { afterEach, describe, expect, it } from "vitest";
import {
  detectImageFormats,
  getImageFormats,
  imageFormatsHeaderValue,
  orderRasterCandidates,
  resetImageFormatsCacheForTests,
} from "./imageFormats";

describe("imageFormats", () => {
  afterEach(() => {
    resetImageFormatsCacheForTests();
  });

  it("orders raster candidates by detected preference", () => {
    localStorage.setItem("prairie.imageFormats", "webp,png");
    expect(
      orderRasterCandidates({
        avif: "https://cdn/poster.avif",
        webp: "https://cdn/poster.webp",
        png: "https://cdn/poster.png",
      }),
    ).toEqual(["https://cdn/poster.webp", "https://cdn/poster.png"]);
  });

  it("reads cached formats for the request header", () => {
    localStorage.setItem("prairie.imageFormats", "avif,webp,png");
    expect(imageFormatsHeaderValue()).toBe("avif,webp,png");
    expect(getImageFormats()).toEqual(["avif", "webp", "png"]);
  });

  it("detects and persists formats", async () => {
    const formats = await detectImageFormats();
    expect(formats.length).toBeGreaterThan(0);
    expect(formats).toContain("png");
    expect(localStorage.getItem("prairie.imageFormats")).toBe(formats.join(","));
  });
});
