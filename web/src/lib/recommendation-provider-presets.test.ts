import { describe, expect, it } from "vitest";

import {
  RECOMMENDATION_PROVIDER_OPTIONS,
  matchRecommendationProviderPreset,
} from "./recommendation-provider-presets";

describe("recommendation provider presets", () => {
  it("includes the wizard custom option alongside the built-in providers", () => {
    expect(RECOMMENDATION_PROVIDER_OPTIONS.map((preset) => preset.id)).toEqual([
      "gemini",
      "ollama",
      "openai",
      "custom",
    ]);
  });

  it("matches saved embedding settings back to a built-in provider", () => {
    expect(
      matchRecommendationProviderPreset(
        "https://api.openai.com/",
        "text-embedding-3-large",
      )?.id,
    ).toBe("openai");
  });

  it("normalizes provider URLs before matching presets", () => {
    expect(
      matchRecommendationProviderPreset(
        " HTTPS://GENERATIVELANGUAGE.GOOGLEAPIS.COM/// ",
        " gemini-embedding-001 ",
      )?.id,
    ).toBe("gemini");
  });

  it("recommends the lightweight Qwen3 Embedding 0.6B Ollama tag", () => {
    const ollama = RECOMMENDATION_PROVIDER_OPTIONS.find(
      (preset) => preset.id === "ollama",
    );
    expect(ollama?.model).toBe("qwen3-embedding:0.6b");
  });

  it("keeps Custom blank but suggests a lightweight LM Studio-style model", () => {
    const custom = RECOMMENDATION_PROVIDER_OPTIONS.find(
      (preset) => preset.id === "custom",
    );
    expect(custom?.baseUrl).toBe("");
    expect(custom?.model).toBe("");
    expect(custom?.urlPlaceholder).toBe("http://host.docker.internal:1234");
    expect(custom?.modelPlaceholder).toBe(
      "text-embedding-qwen3-embedding-0.6b",
    );
  });

  it("returns null when the settings do not match a built-in provider", () => {
    expect(
      matchRecommendationProviderPreset(
        "http://localhost:9999",
        "custom-model",
      ),
    ).toBeNull();
  });

  it("returns null for incomplete provider settings", () => {
    expect(
      matchRecommendationProviderPreset(undefined, "text-embedding-3-large"),
    ).toBeNull();
    expect(
      matchRecommendationProviderPreset("https://api.openai.com", null),
    ).toBeNull();
    expect(
      matchRecommendationProviderPreset("   ", "text-embedding-3-large"),
    ).toBeNull();
  });
});
