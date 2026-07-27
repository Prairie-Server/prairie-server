import { useMemo } from "react";
import { useSettingsForm } from "@/hooks/useSettingsForm";
import { SettingField } from "./SettingField";
import { SaveBar } from "./SaveBar";
import { FieldGroup } from "./FieldGroup";
import { Skeleton } from "@/components/ui/skeleton";

const KEYS = [
  "scanner.workers",
  "matcher.workers",
  "matcher.batch_size",
  "metadata.cache_images",
  "metadata.avif_backfill_workers",
  "metadata.avif_encoder",
  "metadata.avif_ffmpeg_path",
  "metadata.avif_nvenc_sessions",
];

export default function ScannerSettings() {
  const form = useSettingsForm({ keys: useMemo(() => KEYS, []) });

  if (form.isLoading)
    return (
      <div className="space-y-6" role="status" aria-label="Loading settings">
        <Skeleton className="h-8 w-48" />
        <div className="space-y-4">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
        <div className="space-y-4">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
        <div className="space-y-4">
          <Skeleton className="h-10 w-full" />
        </div>
        <span className="sr-only">Loading settings</span>
      </div>
    );

  return (
    <div className="flex h-full flex-col">
      <div className="mb-6 space-y-2">
        <h2 className="text-xl font-semibold tracking-tight">Scanner & Matcher</h2>
        <p className="text-muted-foreground text-sm leading-relaxed">
          Configure scanner performance and metadata matching. Startup and recurring scans are
          managed in Scheduled Tasks.
        </p>
      </div>

      <div className="flex-1 space-y-6">
        <FieldGroup label="Scanner">
          <SettingField
            label="Scanner Workers"
            type="number"
            value={form.getValue("scanner.workers")}
            onChange={(v) => form.setValue("scanner.workers", v)}
          />
        </FieldGroup>

        <FieldGroup label="Matcher">
          <SettingField
            label="Matcher Workers"
            type="number"
            value={form.getValue("matcher.workers")}
            onChange={(v) => form.setValue("matcher.workers", v)}
          />
          <SettingField
            label="Matcher Batch Size"
            type="number"
            value={form.getValue("matcher.batch_size")}
            onChange={(v) => form.setValue("matcher.batch_size", v)}
          />
        </FieldGroup>

        <FieldGroup label="Metadata">
          <SettingField
            label="Cache Artwork"
            type="toggle"
            hint="Download artwork from metadata providers and store resized WebP/AVIF/PNG variants. Uses public S3 when configured; otherwise stores on the local artwork volume."
            value={form.getValue("metadata.cache_images")}
            onChange={(v) => form.setValue("metadata.cache_images", v)}
          />
          <SettingField
            label="AVIF Backfill Workers"
            type="number"
            hint="Concurrent AVIF encodes for deferred sibling backfill. 0 = auto (CPU cores for svt/wasm, NVENC session cap for nvenc). Shared with WebP encodes so a 4-core node is not oversubscribed."
            value={form.getValue("metadata.avif_backfill_workers")}
            onChange={(v) => form.setValue("metadata.avif_backfill_workers", v)}
          />
          <SettingField
            label="AVIF Encoder"
            type="select"
            hint="Still-image AVIF backend. auto picks NVENC (Ada+ AV1) when available, else native SVT-AV1 via ffmpeg, else legacy WASM rav1e. Native SVT requires the debian ffmpeg package (libsvtav1)."
            value={form.getValue("metadata.avif_encoder")}
            onChange={(v) => form.setValue("metadata.avif_encoder", v)}
            options={[
              { value: "auto", label: "Auto (recommended)" },
              { value: "svt", label: "SVT-AV1 (CPU)" },
              { value: "nvenc", label: "NVENC AV1 (GPU, fallback to SVT)" },
              { value: "wasm", label: "WASM rav1e (legacy)" },
            ]}
          />
          <SettingField
            label="AVIF FFmpeg Path"
            type="text"
            hint="ffmpeg binary for SVT/NVENC still encodes (must include libsvtav1 + avif muxer). Defaults to ffmpeg on PATH — not jellyfin-ffmpeg."
            value={form.getValue("metadata.avif_ffmpeg_path")}
            onChange={(v) => form.setValue("metadata.avif_ffmpeg_path", v)}
          />
          <SettingField
            label="AVIF NVENC Sessions"
            type="number"
            hint="Max concurrent NVENC still encodes when the nvenc backend is active. 0 = 3. Tiny display widths still fall back to SVT."
            value={form.getValue("metadata.avif_nvenc_sessions")}
            onChange={(v) => form.setValue("metadata.avif_nvenc_sessions", v)}
          />
        </FieldGroup>
      </div>

      <SaveBar
        dirtyCount={form.dirtyCount}
        onSave={form.save}
        onDiscard={form.discard}
        isSaving={form.isSaving}
        restartRequired={form.restartRequired}
      />
    </div>
  );
}
