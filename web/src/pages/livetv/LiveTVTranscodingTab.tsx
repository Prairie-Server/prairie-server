import { useMemo } from "react";
import { Badge } from "@/components/ui/badge";
import { useLiveTVTuners } from "@/hooks/queries/useLiveTV";
import { useSettingsForm } from "@/hooks/useSettingsForm";
import { FieldGroup } from "@/pages/admin-settings/FieldGroup";
import { SaveBar } from "@/pages/admin-settings/SaveBar";
import { SettingField } from "@/pages/admin-settings/SettingField";

const KEYS = [
  "livetv.hw_accel",
  "livetv.hw_decode",
  "livetv.encoder_preset",
  "livetv.framerate_cap",
  "livetv.max_resolution",
  "livetv.play_method",
  "livetv.max_transcodes",
];

/**
 * Live TV transcoding policy. Broadcast is MPEG-2 with AC-3 audio, which native
 * TV players decode directly but browsers cannot, so those sessions are
 * re-encoded — and an encode that falls below realtime starves the player.
 * These settings exist to get the encode onto the GPU and keep it there.
 */
export function LiveTVTranscodingTab() {
  const form = useSettingsForm({ keys: useMemo(() => KEYS, []) });
  const tuners = useLiveTVTuners();

  // Device-side transcoding shipped only on the discontinued HDHomeRun EXTEND.
  // Current tuners ignore ?transcode= silently, so the option is shown as
  // unavailable rather than offered and quietly dropped.
  const deviceTranscodeTuners = (tuners.data ?? []).filter(
    (tuner) => (tuner.transcode_codecs ?? []).length > 0,
  );

  if (form.isLoading)
    return <p className="text-muted-foreground text-sm">Loading settings…</p>;

  return (
    <div className="flex h-full flex-col">
      <div className="mb-6 space-y-2">
        <h2 className="text-xl font-semibold tracking-tight">Transcoding</h2>
        <p className="text-muted-foreground max-w-2xl text-sm leading-relaxed">
          How Prairie delivers live channels. Clients that can decode the
          broadcast get it untouched; everything else is re-encoded, which needs
          to keep pace with the feed.
        </p>
      </div>

      <div className="flex-1 space-y-6">
        <FieldGroup label="Hardware">
          <SettingField
            label="Hardware acceleration"
            type="select"
            hint="Encoder used for live transcodes. Auto probes this host."
            options={[
              { value: "auto", label: "Auto" },
              { value: "nvenc", label: "NVIDIA NVENC" },
              { value: "qsv", label: "Intel Quick Sync (QSV)" },
              { value: "vaapi", label: "VA-API" },
              { value: "none", label: "Software" },
            ]}
            value={form.getValue("livetv.hw_accel")}
            onChange={(v) => form.setValue("livetv.hw_accel", v)}
          />
          <SettingField
            label="Hardware decoding"
            type="select"
            hint="Decode the broadcast on the GPU. Software MPEG-2 decode at 59.94fps is the usual reason a live transcode cannot hold realtime. Auto uses it when this ffmpeg build has the decoder."
            options={[
              { value: "auto", label: "Auto" },
              { value: "on", label: "Always on" },
              { value: "off", label: "Off (software decode)" },
            ]}
            value={form.getValue("livetv.hw_decode")}
            onChange={(v) => form.setValue("livetv.hw_decode", v)}
          />
          <SettingField
            label="Encoder preset"
            type="select"
            hint="Low latency keeps up with the feed; quality compresses harder and risks falling behind."
            options={[
              { value: "low_latency", label: "Low latency (recommended)" },
              { value: "balanced", label: "Balanced" },
              { value: "quality", label: "Quality" },
            ]}
            value={form.getValue("livetv.encoder_preset")}
            onChange={(v) => form.setValue("livetv.encoder_preset", v)}
          />
          <SettingField
            label="Concurrent transcodes"
            type="number"
            hint="Maximum live sessions that re-encode at once. Copy sessions are not counted. 0 uses the default of 3; -1 removes the limit."
            value={form.getValue("livetv.max_transcodes")}
            onChange={(v) => form.setValue("livetv.max_transcodes", v)}
          />
        </FieldGroup>

        <FieldGroup label="Output">
          <SettingField
            label="Frame rate cap"
            type="select"
            hint="Fallback for hardware that cannot sustain the broadcast rate. Halves motion smoothness, so leave on Source unless transcodes run below realtime."
            options={[
              { value: "source", label: "Source (recommended)" },
              { value: "60", label: "60fps" },
              { value: "30", label: "30fps" },
            ]}
            value={form.getValue("livetv.framerate_cap")}
            onChange={(v) => form.setValue("livetv.framerate_cap", v)}
          />
          <SettingField
            label="Maximum resolution"
            type="select"
            hint="Caps re-encoded video. Never upscales: a ceiling at or above the broadcast is ignored."
            options={[
              { value: "source", label: "Source" },
              { value: "1080p", label: "1080p" },
              { value: "720p", label: "720p" },
            ]}
            value={form.getValue("livetv.max_resolution")}
            onChange={(v) => form.setValue("livetv.max_resolution", v)}
          />
          <SettingField
            label="Play method"
            type="select"
            hint="Auto follows what each client says it can decode. Force copy sends the broadcast untouched (browsers will fail); force transcode re-encodes for everyone."
            options={[
              { value: "auto", label: "Auto (recommended)" },
              { value: "copy", label: "Force copy" },
              { value: "transcode", label: "Force transcode" },
            ]}
            value={form.getValue("livetv.play_method")}
            onChange={(v) => form.setValue("livetv.play_method", v)}
          />
        </FieldGroup>

        <FieldGroup label="Tuner transcoding">
          <div className="space-y-3 py-2">
            <p className="text-muted-foreground text-sm leading-relaxed">
              Some discontinued HDHomeRun models (EXTEND) could transcode on the
              device itself. Prairie only uses it when a tuner advertises the
              capability — current models ignore the request and keep sending
              MPEG-2.
            </p>
            {tuners.isLoading ? (
              <p className="text-muted-foreground text-sm">Checking tuners…</p>
            ) : (tuners.data ?? []).length === 0 ? (
              <p className="text-muted-foreground text-sm">
                No tuners configured.
              </p>
            ) : (
              <ul className="divide-border divide-y border-y">
                {(tuners.data ?? []).map((tuner) => {
                  const profiles = tuner.transcode_codecs ?? [];
                  return (
                    <li
                      key={tuner.id}
                      className="flex flex-wrap items-center justify-between gap-2 py-2"
                    >
                      <span className="text-sm font-medium">
                        {tuner.model || tuner.device_id || tuner.id}
                      </span>
                      {profiles.length > 0 ? (
                        <span className="flex flex-wrap items-center gap-1.5">
                          {profiles.map((profile) => (
                            <Badge key={profile} variant="secondary">
                              {profile}
                            </Badge>
                          ))}
                        </span>
                      ) : (
                        <Badge
                          variant="outline"
                          className="text-muted-foreground"
                        >
                          Not supported by this tuner
                        </Badge>
                      )}
                    </li>
                  );
                })}
              </ul>
            )}
            {deviceTranscodeTuners.length === 0 &&
              (tuners.data ?? []).length > 0 && (
                <p className="text-muted-foreground text-xs">
                  No tuner reports device-side transcoding, so Prairie
                  transcodes on the server.
                </p>
              )}
          </div>
        </FieldGroup>
      </div>

      <SaveBar
        dirtyCount={form.dirtyCount}
        onSave={form.save}
        onDiscard={form.discard}
        isSaving={form.isSaving}
      />
    </div>
  );
}
