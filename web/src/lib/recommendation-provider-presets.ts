export interface RecommendationProviderPreset {
  id: string;
  label: string;
  tag?: string;
  description: string;
  baseUrl: string;
  model: string;
  needsToken: boolean;
  /** Shown in empty inputs when this preset is selected (wizard). */
  urlPlaceholder?: string;
  /** Shown in empty inputs when this preset is selected (wizard). */
  modelPlaceholder?: string;
}

export const RECOMMENDATION_PROVIDER_PRESETS: RecommendationProviderPreset[] = [
  {
    id: "gemini",
    label: "Gemini",
    tag: "Recommended",
    description: "Most accurate. Requires a Google AI API key.",
    baseUrl: "https://generativelanguage.googleapis.com",
    model: "gemini-embedding-001",
    needsToken: true,
  },
  {
    id: "ollama",
    label: "Ollama",
    tag: "Local",
    description: "Free, self-hosted. Needs Ollama running. Uses Qwen3 Embedding 0.6B (~640MB).",
    baseUrl: "http://ollama:11434",
    // Prefer the 0.6B tag: :latest is the 8B (~4.7GB) model and emits 4096-d
    // vectors, which exceed Prairie's 3072 canonical storage width.
    model: "qwen3-embedding:0.6b",
    needsToken: false,
  },
  {
    id: "openai",
    label: "OpenAI",
    description: "High quality. Requires an OpenAI API key.",
    baseUrl: "https://api.openai.com",
    model: "text-embedding-3-large",
    needsToken: true,
  },
];

export const RECOMMENDATION_CUSTOM_PROVIDER_PRESET: RecommendationProviderPreset = {
  id: "custom",
  label: "Custom",
  description: "Any OpenAI-compatible endpoint (LM Studio, vLLM, etc.). Example model uses ~640MB.",
  baseUrl: "",
  model: "",
  needsToken: false,
  urlPlaceholder: "http://host.docker.internal:1234",
  modelPlaceholder: "text-embedding-qwen3-embedding-0.6b",
};

export const RECOMMENDATION_PROVIDER_OPTIONS: RecommendationProviderPreset[] = [
  ...RECOMMENDATION_PROVIDER_PRESETS,
  RECOMMENDATION_CUSTOM_PROVIDER_PRESET,
];

export function matchRecommendationProviderPreset(
  baseUrl: string | null | undefined,
  model: string | null | undefined,
): RecommendationProviderPreset | null {
  const normalizedBaseUrl = normalizeBaseUrl(baseUrl);
  const normalizedModel = model?.trim() ?? "";

  if (!normalizedBaseUrl || !normalizedModel) {
    return null;
  }

  return (
    RECOMMENDATION_PROVIDER_PRESETS.find(
      (preset) =>
        normalizeBaseUrl(preset.baseUrl) === normalizedBaseUrl && preset.model === normalizedModel,
    ) ?? null
  );
}

function normalizeBaseUrl(value: string | null | undefined): string {
  return (value ?? "").trim().replace(/\/+$/, "").toLowerCase();
}
