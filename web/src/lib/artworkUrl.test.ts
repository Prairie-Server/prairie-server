import { describe, expect, it } from "vitest";
import {
  artworkCandidates,
  artworkSrcSet,
  artworkWidthVariant,
  isSignedArtworkURL,
  webPAVIFSibling,
  webPPNGSibling,
} from "./artworkUrl";

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

  it("returns empty for non-WebP inputs", () => {
    expect(webPAVIFSibling("poster.jpg")).toBe("");
    expect(webPAVIFSibling("https://cdn.example.com/art/original.png")).toBe("");
    expect(webPAVIFSibling("")).toBe("");
    expect(webPAVIFSibling(null)).toBe("");
    expect(webPAVIFSibling(undefined)).toBe("");
  });

  it("is case-insensitive on the WebP extension", () => {
    expect(webPAVIFSibling("poster.WEBP")).toBe("poster.avif");
  });

  it("returns empty for malformed absolute URLs", () => {
    expect(webPAVIFSibling("https://[::1")).toBe("");
    expect(webPPNGSibling("not-a-url://poster.webp")).toBe("");
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

  it("returns empty for non-WebP inputs", () => {
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

  it("returns the original URL alone when it is not WebP", () => {
    expect(artworkCandidates("/art/cover.jpg")).toEqual(["/art/cover.jpg"]);
  });

  it("returns empty for blank input", () => {
    expect(artworkCandidates("")).toEqual([]);
    expect(artworkCandidates(null)).toEqual([]);
  });

  it("returns only the original URL for signed artwork", () => {
    const signed = "https://cdn.example.com/art/w300.webp?X-Amz-Signature=abc";
    expect(artworkCandidates(signed)).toEqual([signed]);
  });
});

describe("artworkWidthVariant", () => {
  it("rewrites original and wN variants", () => {
    expect(artworkWidthVariant("/art/poster/original.rev.webp", 500)).toBe(
      "/art/poster/w500.rev.webp",
    );
    expect(artworkWidthVariant("/art/poster/w300.webp", 500)).toBe("/art/poster/w500.webp");
  });

  it("preserves query strings on absolute public URLs", () => {
    expect(artworkWidthVariant("https://cdn.example.com/art/w300.rev.webp?v=1", 1920)).toBe(
      "https://cdn.example.com/art/w1920.rev.webp?v=1",
    );
  });

  it("skips signed URLs", () => {
    expect(
      artworkWidthVariant("https://cdn.example.com/art/w300.webp?X-Amz-Signature=abc", 500),
    ).toBe("");
  });

  it("returns empty for unrecognized paths", () => {
    expect(artworkWidthVariant("/static/logo.png", 500)).toBe("");
    expect(artworkWidthVariant("", 500)).toBe("");
  });
});

describe("artworkSrcSet", () => {
  it("builds a multi-width srcSet", () => {
    expect(artworkSrcSet("/art/poster/w300.webp", [300, 500])).toBe(
      "/art/poster/w300.webp 300w, /art/poster/w500.webp 500w",
    );
  });

  it("returns empty for signed URLs or single candidates", () => {
    expect(artworkSrcSet("/art/poster/w300.webp?X-Amz-Signature=x", [300, 500])).toBe("");
    expect(artworkSrcSet("/art/poster/w300.webp", [300])).toBe("");
    expect(isSignedArtworkURL("https://x/?Signature=1")).toBe(true);
  });
});
