import { useState, useEffect } from "react";
import { toast } from "sonner";
import {
  useAdminSensitiveStatus,
  useAdminServerSettings,
  useUpdateServerSettings,
} from "@/hooks/queries/admin/settings";
import { AlertTriangle, Loader2, Save } from "lucide-react";

import { Button } from "@/components/ui/button";
import { QUOTA_PERIODS, QUOTA_PERIOD_WINDOW_LABELS } from "@/lib/quotaPeriods";
import { CredentialStatus } from "./CredentialStatus";
import { RestartServerButton } from "./RestartServerButton";
import { SettingField } from "./SettingField";

// Connection settings live under the ai.* keys; reads fall back to the legacy
// subtitle_ai.* rows (mirroring the server's loader) so an existing setup
// shows its effective values, while saves always write the new keys.

// Chat-only gateways have no timestamped transcription API; the server
// rejects them for the transcription URL — mirror that check for instant
// feedback. Keep in sync with llm.IsChatOnlyGateway.
const CHAT_ONLY_GATEWAY_HOSTS = ["openrouter.ai"];

function isChatOnlyGateway(rawUrl: string): boolean {
  const trimmed = rawUrl.trim();
  if (!trimmed) return false;
  try {
    const host = new URL(
      trimmed.includes("://") ? trimmed : `https://${trimmed}`,
    ).hostname.toLowerCase();
    return CHAT_ONLY_GATEWAY_HOSTS.some((g) => host === g || host.endsWith(`.${g}`));
  } catch {
    return false;
  }
}

// Recommended transcription endpoints, fastest path first. Clicking a preset
// fills the URL + model; the API key still comes from the operator.
const TRANSCRIPTION_PRESETS: {
  id: string;
  label: string;
  description: string;
  baseUrl: string;
  model: string;
}[] = [
  {
    id: "local",
    label: "Self-hosted · recommended",
    description:
      "A speaches/faster-whisper server on your own hardware — private, free, and no rate limits. Adjust the URL to where it runs; no API key needed.",
    baseUrl: "http://localhost:8000",
    model: "deepdml/faster-whisper-large-v3-turbo-ct2",
  },
  {
    id: "groq-turbo",
    label: "Groq · hosted fallback",
    description:
      "whisper-large-v3-turbo on Groq — fastest hosted option, very low cost (free tier covers ~2 audio-hours per hour). Needs a Groq API key in the transcription key field.",
    baseUrl: "https://api.groq.com/openai",
    model: "whisper-large-v3-turbo",
  },
  {
    id: "groq-accurate",
    label: "Groq · most accurate",
    description:
      "whisper-large-v3 on Groq — best multilingual accuracy among hosted options, slightly slower and pricier than turbo.",
    baseUrl: "https://api.groq.com/openai",
    model: "whisper-large-v3",
  },
  {
    id: "openai",
    label: "OpenAI",
    description:
      "whisper-1 on OpenAI — solid quality, higher cost than Groq. Uses the main API key if the transcription key is blank.",
    baseUrl: "https://api.openai.com",
    model: "whisper-1",
  },
];

function AIConnectionCard() {
  const { data: settings } = useAdminServerSettings();
  const { data: sensitive } = useAdminSensitiveStatus();
  const updateSettings = useUpdateServerSettings();

  const configuredKeys = new Set(sensitive?.configured ?? []);
  const apiKeyConfigured =
    configuredKeys.has("ai.api_key") || configuredKeys.has("subtitle_ai.api_key");
  const asrApiKeyConfigured = configuredKeys.has("ai.asr_api_key");

  const [baseUrl, setBaseUrl] = useState("");
  const [chatModel, setChatModel] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [asrModel, setAsrModel] = useState("");
  const [asrBaseUrl, setAsrBaseUrl] = useState("");
  const [asrApiKey, setAsrApiKey] = useState("");
  const [maxConcurrent, setMaxConcurrent] = useState("2");
  const [restartRequired, setRestartRequired] = useState(false);

  useEffect(() => {
    if (!settings) return;
    setBaseUrl(
      settings["ai.base_url"] ?? settings["subtitle_ai.base_url"] ?? "https://api.openai.com",
    );
    setChatModel(settings["ai.chat_model"] ?? settings["subtitle_ai.chat_model"] ?? "gpt-4o-mini");
    setAsrModel(settings["ai.asr_model"] ?? "whisper-1");
    setAsrBaseUrl(settings["ai.asr_base_url"] ?? "");
    setMaxConcurrent(
      settings["ai.max_concurrent_jobs"] ?? settings["subtitle_ai.max_concurrent_jobs"] ?? "2",
    );
  }, [settings]);

  async function save() {
    const trimmedBaseUrl = baseUrl.trim();
    const trimmedChatModel = chatModel.trim();
    const parsedMaxConcurrent = Number.parseInt(maxConcurrent, 10);

    if (trimmedBaseUrl === "" || trimmedChatModel === "") {
      toast.error("Base URL and chat model are required.");
      return;
    }
    if (!Number.isInteger(parsedMaxConcurrent) || parsedMaxConcurrent < 1) {
      toast.error("Max concurrent jobs must be a positive whole number.");
      return;
    }
    if (isChatOnlyGateway(asrBaseUrl)) {
      toast.error(
        "That endpoint can't produce timestamped transcriptions (chat-only gateway). Pick a transcription preset below or use a Whisper-capable server.",
      );
      return;
    }

    const candidates: Record<string, string> = {
      "ai.base_url": trimmedBaseUrl,
      "ai.chat_model": trimmedChatModel,
      "ai.asr_model": asrModel.trim(),
      "ai.asr_base_url": asrBaseUrl.trim(),
      "ai.max_concurrent_jobs": String(parsedMaxConcurrent),
    };
    const updates = Object.fromEntries(
      Object.entries(candidates).filter(([key, value]) => settings?.[key] !== value),
    );
    if (apiKey.trim() !== "") {
      updates["ai.api_key"] = apiKey;
    }
    if (asrApiKey.trim() !== "") {
      updates["ai.asr_api_key"] = asrApiKey;
    }
    if (Object.keys(updates).length === 0) {
      toast.info("No endpoint settings changed.");
      return;
    }
    try {
      const result = await updateSettings.mutateAsync(updates);
      setApiKey("");
      setAsrApiKey("");
      setRestartRequired((current) => current || result.restart_required);
      toast.success("AI endpoint settings saved");
    } catch {
      // The mutation reports the API error.
    }
  }

  return (
    <fieldset
      disabled={updateSettings.isPending}
      className="border-border bg-surface max-w-2xl rounded-lg border px-5 py-4"
    >
      <div className="mb-2 flex items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold">Endpoint</h3>
          <p className="text-muted-foreground text-xs">
            One OpenAI-compatible endpoint shared by every AI feature (OpenAI, Groq, a local Ollama
            server, …). Transcription can use a separate Whisper-compatible server.
          </p>
        </div>
        <CredentialStatus configured={apiKeyConfigured} />
      </div>
      <SettingField
        label="Base URL"
        type="text"
        value={baseUrl}
        onChange={setBaseUrl}
        hint="https://api.openai.com"
      />
      <SettingField
        label="Chat model"
        type="text"
        value={chatModel}
        onChange={setChatModel}
        hint="Used for subtitle and description translation, e.g. gpt-4o-mini, llama3.1"
      />
      <SettingField
        label="API Key"
        type="password"
        value={apiKey}
        onChange={setApiKey}
        sensitiveConfigured={apiKeyConfigured}
        hint="Leave blank to keep current. Empty is fine for keyless local servers."
      />
      <div className="border-border/60 mt-4 mb-1 border-t pt-3">
        <p className="text-sm font-medium">Transcription</p>
        <p className="text-muted-foreground text-xs">
          Subtitle generation needs a Whisper endpoint that returns segment timestamps. Pick a
          preset or configure your own:
        </p>
        <div className="mt-2 flex flex-wrap gap-2">
          {TRANSCRIPTION_PRESETS.map((preset) => {
            const active = asrBaseUrl.trim() === preset.baseUrl && asrModel.trim() === preset.model;
            return (
              <button
                key={preset.id}
                type="button"
                title={preset.description}
                onClick={() => {
                  setAsrBaseUrl(preset.baseUrl);
                  setAsrModel(preset.model);
                }}
                className={`rounded-full border px-3 py-1 text-xs transition-colors ${
                  active
                    ? "border-primary bg-primary/10 text-primary"
                    : "border-border text-muted-foreground hover:text-foreground"
                }`}
              >
                {preset.label}
              </button>
            );
          })}
        </div>
      </div>
      <SettingField
        label="Transcription model"
        type="text"
        value={asrModel}
        onChange={setAsrModel}
        hint="Whisper model for subtitle generation, e.g. whisper-large-v3-turbo"
      />
      <SettingField
        label="Transcription base URL"
        type="text"
        value={asrBaseUrl}
        onChange={setAsrBaseUrl}
        hint="Whisper-capable endpoint with segment timestamps: a self-hosted faster-whisper/speaches server (recommended), api.groq.com/openai, or api.openai.com. Blank uses the base URL — note that chat-only gateways (e.g. OpenRouter) cannot transcribe."
      />
      <SettingField
        label="Transcription API key"
        type="password"
        value={asrApiKey}
        onChange={setAsrApiKey}
        sensitiveConfigured={asrApiKeyConfigured}
        hint="Optional; blank uses the main API key."
      />
      <SettingField
        label="Max concurrent jobs"
        type="number"
        value={maxConcurrent}
        onChange={setMaxConcurrent}
        hint="One shared cap across subtitle translation, transcription, and description translation."
      />
      <div className="pt-2">
        <Button type="button" onClick={() => void save()} disabled={updateSettings.isPending}>
          {updateSettings.isPending ? <Loader2 className="animate-spin" /> : <Save />}
          {updateSettings.isPending ? "Saving..." : "Save Endpoint Settings"}
        </Button>
        <p className="text-muted-foreground mt-2 text-xs">
          Endpoint, model, and credential changes apply live. Changing the concurrency cap requires
          a restart.
        </p>
        {restartRequired && (
          <div className="border-warning/30 bg-warning/10 text-warning mt-3 flex items-center justify-between gap-3 rounded-xl border px-3 py-2 text-xs">
            <span className="flex items-center gap-2">
              <AlertTriangle className="h-3.5 w-3.5" />
              Restart required to resize the AI job pool.
            </span>
            <RestartServerButton />
          </div>
        )}
      </div>
    </fieldset>
  );
}

function AIFeaturesCard() {
  const { data: settings } = useAdminServerSettings();
  const updateSettings = useUpdateServerSettings();

  const [subtitleTranslate, setSubtitleTranslate] = useState("false");
  const [transcribe, setTranscribe] = useState("false");
  const [metadataTranslate, setMetadataTranslate] = useState("false");
  const [onView, setOnView] = useState("off");
  const [batchSize, setBatchSize] = useState("40");
  const [contextNeighbors, setContextNeighbors] = useState("2");
  const [asrChunkSeconds, setAsrChunkSeconds] = useState("600");
  const [transcribeQuotaJobs, setTranscribeQuotaJobs] = useState("0");
  const [transcribeQuotaPeriod, setTranscribeQuotaPeriod] = useState("day");

  useEffect(() => {
    if (!settings) return;
    setSubtitleTranslate(settings["subtitle_ai.enabled"] ?? "false");
    setTranscribe(settings["subtitle_ai.transcribe_enabled"] ?? "false");
    setMetadataTranslate(settings["metadata_ai.enabled"] ?? "false");
    setOnView(settings["metadata_ai.on_view"] ?? "off");
    setBatchSize(settings["subtitle_ai.batch_size"] ?? "40");
    setContextNeighbors(settings["subtitle_ai.context_neighbors"] ?? "2");
    setAsrChunkSeconds(settings["subtitle_ai.asr_chunk_seconds"] ?? "600");
    setTranscribeQuotaJobs(settings["subtitle_ai.transcribe_quota_jobs"] ?? "0");
    setTranscribeQuotaPeriod(settings["subtitle_ai.transcribe_quota_period"] ?? "day");
  }, [settings]);

  async function save() {
    const parsedBatch = Number.parseInt(batchSize, 10);
    const parsedNeighbors = Number.parseInt(contextNeighbors, 10);
    if (!Number.isInteger(parsedBatch) || parsedBatch < 1) {
      toast.error("Batch size must be a positive whole number.");
      return;
    }
    if (!Number.isInteger(parsedNeighbors) || parsedNeighbors < 0) {
      toast.error("Context lines must be zero or a positive whole number.");
      return;
    }
    const parsedChunkSeconds = Number.parseInt(asrChunkSeconds, 10);
    if (
      !Number.isInteger(parsedChunkSeconds) ||
      parsedChunkSeconds < 60 ||
      parsedChunkSeconds > 600
    ) {
      toast.error("Transcription chunk length must be between 60 and 600 seconds.");
      return;
    }
    const parsedQuotaJobs = Number.parseInt(transcribeQuotaJobs, 10);
    if (!Number.isInteger(parsedQuotaJobs) || parsedQuotaJobs < 0) {
      toast.error("Transcription limit must be zero (unlimited) or a positive whole number.");
      return;
    }
    const candidates: Record<string, string> = {
      "subtitle_ai.enabled": subtitleTranslate,
      "subtitle_ai.transcribe_enabled": transcribe,
      "metadata_ai.enabled": metadataTranslate,
      "metadata_ai.on_view": onView,
      "subtitle_ai.batch_size": String(parsedBatch),
      "subtitle_ai.context_neighbors": String(parsedNeighbors),
      "subtitle_ai.asr_chunk_seconds": String(parsedChunkSeconds),
      "subtitle_ai.transcribe_quota_jobs": String(parsedQuotaJobs),
      "subtitle_ai.transcribe_quota_period": transcribeQuotaPeriod,
    };
    const updates = Object.fromEntries(
      Object.entries(candidates).filter(([key, value]) => settings?.[key] !== value),
    );
    if (Object.keys(updates).length === 0) {
      toast.info("No feature settings changed.");
      return;
    }
    try {
      await updateSettings.mutateAsync(updates);
      toast.success("AI feature settings saved");
    } catch {
      // The mutation reports the API error.
    }
  }

  return (
    <fieldset
      disabled={updateSettings.isPending}
      className="border-border bg-surface max-w-2xl rounded-lg border px-5 py-4"
    >
      <div className="mb-2">
        <h3 className="text-sm font-semibold">Features</h3>
        <p className="text-muted-foreground text-xs">
          Everything runs once on the server and is served to every client through the normal
          subtitle and metadata pipelines.
        </p>
      </div>
      <SettingField
        label="Subtitle translation"
        type="toggle"
        value={subtitleTranslate}
        onChange={setSubtitleTranslate}
        hint="Show the “Translate with AI” action in the player."
      />
      <SettingField
        label="Subtitle generation from audio"
        type="toggle"
        value={transcribe}
        onChange={setTranscribe}
        hint="Whisper transcription — generates subtitle tracks for media with no usable text subtitles."
      />
      <SettingField
        label="Description translation"
        type="toggle"
        value={metadataTranslate}
        onChange={setMetadataTranslate}
        hint="Translate overviews and taglines from the metadata editor, plus the per-library auto-translate option."
      />
      <SettingField
        label="On-view translation"
        type="select"
        value={onView}
        onChange={setOnView}
        options={[
          { value: "off", label: "Off" },
          { value: "button", label: "Translate button on detail pages" },
          { value: "auto", label: "Automatic on view" },
        ]}
        hint="Let viewers get descriptions in their profile's metadata language: a Translate button, or automatic translation when they open a detail page. Requires description translation."
      />
      <SettingField
        label="Subtitle batch size"
        type="number"
        value={batchSize}
        onChange={setBatchSize}
        hint="Cues per translation request."
      />
      <SettingField
        label="Subtitle context lines"
        type="number"
        value={contextNeighbors}
        onChange={setContextNeighbors}
        hint="Preceding source cues sent for scene continuity across batches."
      />
      <SettingField
        label="Transcription chunk length (seconds)"
        type="number"
        value={asrChunkSeconds}
        onChange={setAsrChunkSeconds}
        hint="60–600. Shorter chunks keep Whisper timestamps tighter on long files (try 300 if subtitles drift), at the cost of more requests and occasional clipped words at chunk boundaries."
      />
      <SettingField
        label="Transcription limit per account"
        type="number"
        value={transcribeQuotaJobs}
        onChange={setTranscribeQuotaJobs}
        hint="Maximum transcription jobs per user account each period; profiles on an account share the limit. The admin account's primary profile is exempt. 0 = unlimited. Jobs that fail before transcribing anything don't count."
      />
      <SettingField
        label="Transcription limit period"
        type="select"
        value={transcribeQuotaPeriod}
        onChange={setTranscribeQuotaPeriod}
        options={QUOTA_PERIODS.map((p) => ({
          value: p,
          label: `Per ${p} (rolling ${QUOTA_PERIOD_WINDOW_LABELS[p]})`,
        }))}
        hint="Rolling window the transcription limit counts against."
      />
      <div className="pt-2">
        <Button type="button" onClick={() => void save()} disabled={updateSettings.isPending}>
          {updateSettings.isPending ? <Loader2 className="animate-spin" /> : <Save />}
          {updateSettings.isPending ? "Saving..." : "Save Feature Settings"}
        </Button>
        <p className="text-muted-foreground mt-2 text-xs">Feature changes apply live.</p>
      </div>
    </fieldset>
  );
}

export default function AIServicesSettings() {
  return (
    <div className="flex h-full flex-col">
      <div className="mb-6 space-y-2">
        <h2 className="text-xl font-semibold tracking-tight">AI Services</h2>
        <p className="text-muted-foreground text-sm leading-relaxed">
          Shared AI endpoint and feature toggles for subtitle translation, subtitle generation from
          audio, and description translation.
        </p>
      </div>

      <div className="space-y-8">
        <AIConnectionCard />
        <AIFeaturesCard />
      </div>
    </div>
  );
}
