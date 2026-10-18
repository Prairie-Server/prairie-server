// @vitest-environment jsdom

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { BrandingContext, type BrandingContextValue } from "@/contexts/BrandingProvider";

import { PrairieBrand, type PrairieBrandVariant } from "./PrairieBrand";

const BRANDING_DEFAULTS: BrandingContextValue = {
  serverName: "Prairie",
  loginSubtitle: "Sign in with an existing account.",
  accentColor: null,
  wordmarkUrl: null,
  markUrl: null,
  faviconUrl: null,
  loginBgUrl: null,
  storageAvailable: false,
  loaded: false,
};

function renderBrandSrc(
  variant: PrairieBrandVariant,
  branding: Partial<BrandingContextValue> = {},
): string | null {
  const markup = renderToStaticMarkup(
    <BrandingContext value={{ ...BRANDING_DEFAULTS, ...branding }}>
      <PrairieBrand variant={variant} />
    </BrandingContext>,
  );
  return new DOMParser()
    .parseFromString(markup, "text/html")
    .querySelector("img")
    ?.getAttribute("src") as string | null;
}

describe("PrairieBrand", () => {
  it("uses the custom wordmark", () => {
    expect(renderBrandSrc("wordmark", { wordmarkUrl: "https://cdn/custom.png" })).toBe(
      "https://cdn/custom.png",
    );
  });

  it("uses the custom mark", () => {
    expect(renderBrandSrc("mark", { markUrl: "https://cdn/mark.png" })).toBe(
      "https://cdn/mark.png",
    );
  });
});
