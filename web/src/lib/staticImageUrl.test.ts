import { beforeEach, describe, expect, it } from "vitest";
import { resetImageFormatsCacheForTests } from "./imageFormats";
import { staticRasterCandidates, staticRasterFormats } from "./staticImageUrl";

describe("staticRasterFormats", () => {
  it("derives avif/webp/png siblings from a png path", () => {
    expect(staticRasterFormats("/prairie-icon-1024.png")).toEqual({
      avif: "/prairie-icon-1024.avif",
      webp: "/prairie-icon-1024.webp",
      png: "/prairie-icon-1024.png",
    });
  });

  it("preserves query strings", () => {
    expect(staticRasterFormats("/prairie-icon-1024.webp?v=2")).toEqual({
      avif: "/prairie-icon-1024.avif?v=2",
      webp: "/prairie-icon-1024.webp?v=2",
      png: "/prairie-icon-1024.png?v=2",
    });
  });

  it("returns null for non-raster paths", () => {
    expect(staticRasterFormats("/favicon.ico")).toBeNull();
    expect(staticRasterFormats("/mark.svg")).toBeNull();
  });
});

describe("staticRasterCandidates", () => {
  beforeEach(() => {
    resetImageFormatsCacheForTests();
    localStorage.setItem("prairie.imageFormats.v2", "avif,webp,png");
  });

  it("orders AVIF → WebP → PNG", () => {
    expect(staticRasterCandidates("/prairie-wordmark-sidebar.png")).toEqual([
      "/prairie-wordmark-sidebar.avif",
      "/prairie-wordmark-sidebar.webp",
      "/prairie-wordmark-sidebar.png",
    ]);
  });

  it("orders AVIF → WebP → PNG from a webp canonical path", () => {
    expect(staticRasterCandidates("/images/collection-templates/trending.webp")).toEqual([
      "/images/collection-templates/trending.avif",
      "/images/collection-templates/trending.webp",
      "/images/collection-templates/trending.png",
    ]);
  });

  it("respects a WebP-first capability preference", () => {
    resetImageFormatsCacheForTests();
    localStorage.setItem("prairie.imageFormats.v2", "webp,png");
    expect(staticRasterCandidates("/images/collection-templates/trending.webp")).toEqual([
      "/images/collection-templates/trending.webp",
      "/images/collection-templates/trending.png",
    ]);
  });
});
