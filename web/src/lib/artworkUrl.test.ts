import { describe, expect, it, beforeEach } from "vitest";
import { resetImageFormatsCacheForTests } from "./imageFormats";
import {
  artworkCandidates,
  artworkSrcSet,
  artworkWidthVariant,
  isPrairieSignedArtworkURL,
  isSignedArtworkURL,
  isSignedOriginalArtworkURL,
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

  it("returns empty for non-raster inputs", () => {
    expect(webPAVIFSibling("poster.jpg")).toBe("");
    expect(webPAVIFSibling("")).toBe("");
    expect(webPAVIFSibling(null)).toBe("");
    expect(webPAVIFSibling(undefined)).toBe("");
  });

  it("derives AVIF from PNG paths via the shared raster helper", () => {
    expect(webPAVIFSibling("https://cdn.example.com/art/original.png")).toBe(
      "https://cdn.example.com/art/original.avif",
    );
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

  it("returns empty for non-raster inputs", () => {
    expect(webPPNGSibling("poster.jpg")).toBe("");
    expect(webPPNGSibling("")).toBe("");
  });
});

describe("artworkCandidates", () => {
  beforeEach(() => {
    resetImageFormatsCacheForTests();
    localStorage.setItem("prairie.imageFormats.v2", "avif,webp,png");
  });

  it("orders AVIF → WebP → PNG for WebP artwork", () => {
    expect(artworkCandidates("/art/original.rev.webp")).toEqual([
      "/art/original.rev.avif",
      "/art/original.rev.webp",
      "/art/original.rev.png",
    ]);
  });

  it("returns the original URL alone when it is not a raster sibling set", () => {
    expect(artworkCandidates("/art/cover.jpg")).toEqual(["/art/cover.jpg"]);
  });

  it("orders AVIF → WebP → PNG for template webp paths", () => {
    expect(artworkCandidates("/images/collection-templates/trending.webp")).toEqual([
      "/images/collection-templates/trending.avif",
      "/images/collection-templates/trending.webp",
      "/images/collection-templates/trending.png",
    ]);
  });

  it("returns empty for blank input", () => {
    expect(artworkCandidates("")).toEqual([]);
    expect(artworkCandidates(null)).toEqual([]);
  });

  it("returns only the original URL for signed artwork", () => {
    const signed = "https://cdn.example.com/art/w300.webp?X-Amz-Signature=abc";
    expect(artworkCandidates(signed)).toEqual([signed]);
  });

  it("treats Cloudflare verify tokens as signed", () => {
    const signed = "https://cdn.example.com/art/w300.webp?verify=123-abc";
    expect(isSignedArtworkURL(signed)).toBe(true);
    expect(artworkCandidates(signed)).toEqual([signed]);
  });

  it("prefers API-provided signed format siblings", () => {
    const webp = "https://cdn.example.com/art/w300.webp?X-Amz-Signature=webp";
    const avif = "https://cdn.example.com/art/w300.avif?X-Amz-Signature=avif";
    const png = "https://cdn.example.com/art/w300.png?X-Amz-Signature=png";
    expect(artworkCandidates(webp, { avif, png })).toEqual([avif, webp, png]);
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

describe("Prairie's own artwork signature", () => {
  // It covers the artwork revision, not the exact key, so selecting another rung
  // of the same image still validates. Treating it as unrewritable disabled both
  // srcSet and the sizes attribute, leaving 140-160px cards fetching w500.
  it("is rewritable, unlike a third-party signature", () => {
    const signed = "/artwork/tmdb/movies/550/poster/w500.rev.webp?expires=123&sig=abc";
    expect(isPrairieSignedArtworkURL(signed)).toBe(true);
    expect(isSignedArtworkURL(signed)).toBe(false);
  });

  it("only claims URLs carrying the store's full shape", () => {
    // A third-party URL that happens to use "sig" must stay untouched.
    const foreign = "https://cdn.example.com/art/w500.webp?sig=abc";
    expect(isPrairieSignedArtworkURL(foreign)).toBe(false);
    expect(isSignedArtworkURL(foreign)).toBe(true);
    expect(isPrairieSignedArtworkURL("/artwork/x/w500.rev.webp?expires=1")).toBe(false);
    expect(isPrairieSignedArtworkURL("https://x/?X-Amz-Signature=1&expires=1&sig=a")).toBe(false);
  });

  // The signature must travel with every candidate or each one 403s.
  it("yields a srcSet whose entries all keep the query", () => {
    const signed = "/artwork/tmdb/movies/550/poster/w500.rev.webp?expires=123&sig=abc";
    expect(artworkSrcSet(signed, [200, 300, 500])).toBe(
      "/artwork/tmdb/movies/550/poster/w200.rev.webp?expires=123&sig=abc 200w, " +
        "/artwork/tmdb/movies/550/poster/w300.rev.webp?expires=123&sig=abc 300w, " +
        "/artwork/tmdb/movies/550/poster/w500.rev.webp?expires=123&sig=abc 500w",
    );
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

describe("signed original artwork URLs", () => {
  const signedOriginal = "/artwork/people/p1/profile/original.7.webp?sig=abc&expires=99";
  const signedRung = "/artwork/people/p1/profile/w500.7.webp?sig=abc&expires=99";

  // The store signs `original` against exactly itself, so any width rewrite is
  // a 403. Cast portraits hit this: catalog.detail presigned the raw original.
  it("refuses to rewrite the width of a signed original", () => {
    expect(isSignedOriginalArtworkURL(signedOriginal)).toBe(true);
    expect(artworkWidthVariant(signedOriginal, 200)).toBe("");
    expect(artworkSrcSet(signedOriginal, [200, 300, 500])).toBe("");
  });

  it("still rewrites a signed sized rung, which shares the revision scope", () => {
    expect(isSignedOriginalArtworkURL(signedRung)).toBe(false);
    expect(artworkWidthVariant(signedRung, 200)).toContain("/w200.7.webp");
    expect(artworkSrcSet(signedRung, [200, 300, 500])).toContain("200w");
  });

  // An unsigned original (local dev without a URL secret, and the tests) has no
  // signature to invalidate, so the ladder still applies.
  it("leaves unsigned originals rewritable", () => {
    const unsigned = "/artwork/people/p1/profile/original.7.webp";
    expect(isSignedOriginalArtworkURL(unsigned)).toBe(false);
    expect(artworkWidthVariant(unsigned, 200)).toContain("/w200.7.webp");
  });

  it("does not mistake a third-party URL containing 'original' for ours", () => {
    const external = "https://images.example.test/original.jpg?sig=abc&expires=99";
    expect(isSignedOriginalArtworkURL(external)).toBe(false);
  });
});
