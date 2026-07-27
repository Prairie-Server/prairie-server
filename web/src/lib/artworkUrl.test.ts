import { describe, expect, it } from "vitest";
import { artworkCandidates, webPAVIFSibling, webPPNGSibling } from "./artworkUrl";

describe("webPAVIFSibling", () => {
  it("rewrites WebP object keys to AVIF siblings", () => {
    expect(webPAVIFSibling("library/1/poster/original.abc123.webp")).toBe(
      "library/1/poster/original.abc123.avif",
    );
    expect(webPAVIFSibling("original.webp")).toBe("original.avif");
  });

  it("preserves query strings on absolute URLs", () => {
    expect(
      webPAVIFSibling("https://cdn.example.com/art/original.rev.webp?X-Amz-Signature=abc"),
    ).toBe("https://cdn.example.com/art/original.rev.avif?X-Amz-Signature=abc");
  });

  it("derives AVIF from PNG paths via the shared raster helper", () => {
    expect(webPAVIFSibling("https://cdn.example.com/art/original.png")).toBe(
      "https://cdn.example.com/art/original.avif",
    );
  });

  it("returns empty for non-raster inputs", () => {
    expect(webPAVIFSibling("poster.jpg")).toBe("");
    expect(webPAVIFSibling("")).toBe("");
    expect(webPAVIFSibling(null)).toBe("");
    expect(webPAVIFSibling(undefined)).toBe("");
  });

  it("is case-insensitive on the WebP extension", () => {
    expect(webPAVIFSibling("poster.WEBP")).toBe("poster.avif");
  });
});

describe("webPPNGSibling", () => {
  it("rewrites WebP object keys to PNG siblings", () => {
    expect(webPPNGSibling("library/1/poster/original.abc123.webp")).toBe(
      "library/1/poster/original.abc123.png",
    );
  });

  it("preserves query strings on absolute URLs", () => {
    expect(
      webPPNGSibling("https://cdn.example.com/art/original.rev.webp?X-Amz-Signature=abc"),
    ).toBe("https://cdn.example.com/art/original.rev.png?X-Amz-Signature=abc");
  });

  it("returns empty for non-raster inputs", () => {
    expect(webPPNGSibling("poster.jpg")).toBe("");
    expect(webPPNGSibling("")).toBe("");
  });
});

describe("artworkCandidates", () => {
  it("orders AVIF → WebP → PNG for WebP artwork", () => {
    expect(artworkCandidates("/art/original.rev.webp")).toEqual([
      "/art/original.rev.avif",
      "/art/original.rev.webp",
      "/art/original.rev.png",
    ]);
  });

  it("orders AVIF → WebP → PNG for PNG canonical paths too", () => {
    expect(artworkCandidates("/images/collection-templates/trending.webp")).toEqual([
      "/images/collection-templates/trending.avif",
      "/images/collection-templates/trending.webp",
      "/images/collection-templates/trending.png",
    ]);
  });

  it("returns the original URL alone when it is not a raster sibling set", () => {
    expect(artworkCandidates("/art/cover.jpg")).toEqual(["/art/cover.jpg"]);
  });

  it("returns empty for blank input", () => {
    expect(artworkCandidates("")).toEqual([]);
    expect(artworkCandidates(null)).toEqual([]);
  });
});
