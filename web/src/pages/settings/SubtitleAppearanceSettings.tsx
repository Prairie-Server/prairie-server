import { useId, useMemo, useState, type ReactNode } from "react";
import { RotateCcw } from "lucide-react";
import { getProfileToken } from "@/api/client";
import { SettingsGroup } from "@/components/settings/SettingsGroup";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  useDeleteDeviceSetting,
  useEffectiveSettings,
  useSetDeviceSetting,
} from "@/hooks/queries/settings";
import { useUpdateProfile } from "@/hooks/queries/profiles";
import { useAuth } from "@/hooks/useAuth";
import { useCurrentProfile } from "@/hooks/useCurrentProfile";
import { LANGUAGE_OPTIONS } from "@/lib/settingsManifest";
import {
  BACKGROUND_STYLE_OPTIONS,
  BG_COLOR_PALETTE,
  computeSubtitleStyles,
  FONT_COLOR_PALETTE,
  FONT_FAMILY_OPTIONS,
  FONT_SIZE_OPTIONS,
  parseSubtitleAppearance,
  POSITION_OPTIONS,
} from "@/lib/subtitleAppearance";
import type { SubtitleAppearance } from "@/lib/subtitleAppearance";
import { toast } from "sonner";

const SETTINGS_KEY = "subtitle_appearance";
const SUBTITLE_MODES = [
  { value: "auto", label: "Auto" },
  { value: "always", label: "Always" },
  { value: "off", label: "Off" },
] as const;

interface ColorPaletteProps {
  colors: { hex: string; label: string }[];
  selected: string;
  onChange: (hex: string) => void;
  disabled?: boolean;
  labelId?: string;
  descriptionId?: string;
}

function ColorPalette({
  colors,
  selected,
  onChange,
  disabled,
  labelId,
  descriptionId,
}: ColorPaletteProps) {
  return (
    <div
      role="group"
      aria-labelledby={labelId}
      aria-describedby={descriptionId}
      className={`flex flex-wrap gap-2 ${disabled ? "opacity-40" : ""}`}
    >
      {colors.map((color) => (
        <button
          key={color.hex}
          type="button"
          title={color.label}
          aria-label={color.label}
          onClick={() => onChange(color.hex)}
          disabled={disabled}
          className="h-7 w-7 rounded-full border-2 transition-transform hover:scale-110"
          style={{
            backgroundColor: color.hex,
            borderColor: selected === color.hex ? "var(--primary)" : "transparent",
            boxShadow:
              color.hex === "#000000" ? "inset 0 0 0 1px rgba(255,255,255,0.2)" : undefined,
          }}
        />
      ))}
    </div>
  );
}

interface SettingRowProps {
  label: string;
  description?: string;
  labelForControl?: boolean;
  children: (props: { id: string; labelId: string; descriptionId: string }) => ReactNode;
}

function SettingRow({ label, description, labelForControl = true, children }: SettingRowProps) {
  const controlId = useId();
  const labelId = useId();
  const descriptionId = useId();

  return (
    <div className="border-border/50 grid gap-3 border-t pt-4 first:border-t-0 first:pt-0 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <div className="min-w-0 space-y-0.5">
        <Label
          id={labelId}
          htmlFor={labelForControl ? controlId : undefined}
          className="text-sm font-medium"
        >
          {label}
        </Label>
        {description ? (
          <p id={descriptionId} className="text-muted-foreground text-[13px] leading-relaxed">
            {description}
          </p>
        ) : null}
      </div>
      <div className="flex md:justify-end">
        {children({ id: controlId, labelId, descriptionId })}
      </div>
    </div>
  );
}

export default function SubtitleAppearanceSettings() {
  const { selectProfile } = useAuth();
  const { profile } = useCurrentProfile();
  const { data: effective = {} } = useEffectiveSettings(profile?.id, [SETTINGS_KEY]);
  const updateMutation = useUpdateProfile();
  const setDeviceSetting = useSetDeviceSetting();
  const deleteDeviceSetting = useDeleteDeviceSetting();
  const currentProfileToken = getProfileToken() ?? undefined;
  const effectiveEntry = effective[SETTINGS_KEY];

  const effectiveJson = effectiveEntry?.effective_value ?? null;
  const effectiveSettings = useMemo(() => parseSubtitleAppearance(effectiveJson), [effectiveJson]);
  const [draftState, setDraftState] = useState<{
    key: string;
    settings: SubtitleAppearance;
  }>({
    key: JSON.stringify(effectiveSettings),
    settings: effectiveSettings,
  });

  const baselineKey = JSON.stringify(effectiveSettings);
  const settings = draftState.key === baselineKey ? draftState.settings : effectiveSettings;

  function update<K extends keyof SubtitleAppearance>(key: K, value: SubtitleAppearance[K]) {
    setDraftState((prev) => ({
      key: baselineKey,
      settings: {
        ...(prev.key === baselineKey ? prev.settings : effectiveSettings),
        [key]: value,
      },
    }));
  }

  function discardLocalChanges() {
    setDraftState({
      key: baselineKey,
      settings: effectiveSettings,
    });
  }

  function handleSave() {
    setDeviceSetting.mutate(
      { key: SETTINGS_KEY, value: JSON.stringify(settings) },
      {
        onSuccess: () => toast.success("Subtitle appearance saved"),
        onError: () => toast.error("Failed to save subtitle appearance"),
      },
    );
  }

  function handleUseFallback() {
    deleteDeviceSetting.mutate(
      { key: SETTINGS_KEY },
      {
        onSuccess: () => toast.success("Subtitle appearance reset"),
        onError: () => toast.error("Failed to reset subtitle appearance"),
      },
    );
  }

  function saveProfileField(body: {
    subtitle_language?: string;
    subtitle_mode?: string;
    show_forced_subtitles?: boolean;
  }) {
    if (!profile) return;
    updateMutation.mutate(
      { id: profile.id, body },
      {
        onSuccess: (updatedProfile) => selectProfile(updatedProfile, currentProfileToken),
        onError: () => toast.error("Failed to save subtitle setting"),
      },
    );
  }

  const hasUnsavedChanges = JSON.stringify(settings) !== JSON.stringify(effectiveSettings);
  const hasDeviceOverride = effectiveEntry?.has_device_override ?? false;
  const usesTextOutline = settings.textOutline || settings.backgroundStyle === "outline";
  const isBoxStyle = settings.backgroundStyle === "box";
  const { containerStyle, cueStyle } = computeSubtitleStyles(settings);

  return (
    <div className="space-y-6">
      <div className="space-y-4">
        <div className="space-y-3">
          <h2 className="text-2xl font-semibold tracking-tight sm:text-3xl">Subtitles</h2>
          <p className="text-muted-foreground max-w-2xl text-sm leading-relaxed">
            Choose when subtitles appear and how they look during playback.
          </p>
        </div>
      </div>

      <SettingsGroup
        title="Behavior"
        description="These preferences decide which subtitles Prairie chooses by default."
      >
        <SettingRow
          label="Subtitle language"
          description="Pick a subtitle language or leave subtitles off by default."
        >
          {({ id, descriptionId }) => (
            <Select
              value={profile?.subtitle_language || "none"}
              onValueChange={(value) =>
                saveProfileField({ subtitle_language: value === "none" ? "" : value })
              }
            >
              <SelectTrigger
                id={id}
                aria-describedby={descriptionId}
                className="w-full sm:w-[220px]"
                disabled={updateMutation.isPending}
              >
                <SelectValue placeholder="None" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">None</SelectItem>
                {LANGUAGE_OPTIONS.filter((language) => language.value).map((language) => (
                  <SelectItem key={language.value} value={language.value}>
                    {language.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </SettingRow>

        <SettingRow label="Subtitle behavior" description="Decide when subtitles should appear.">
          {({ id, descriptionId }) => (
            <Select
              value={profile?.subtitle_mode || "auto"}
              onValueChange={(value) => saveProfileField({ subtitle_mode: value })}
            >
              <SelectTrigger
                id={id}
                aria-describedby={descriptionId}
                className="w-full sm:w-[220px]"
                disabled={updateMutation.isPending}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SUBTITLE_MODES.map((mode) => (
                  <SelectItem key={mode.value} value={mode.value}>
                    {mode.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </SettingRow>

        <SettingRow
          label="Show forced subtitles"
          description="Display forced subtitles for foreign-language dialogue."
        >
          {({ id, descriptionId }) => (
            <Switch
              id={id}
              aria-describedby={descriptionId}
              checked={profile?.show_forced_subtitles ?? true}
              disabled={updateMutation.isPending}
              onCheckedChange={(checked) => saveProfileField({ show_forced_subtitles: checked })}
            />
          )}
        </SettingRow>
      </SettingsGroup>

      <SettingsGroup
        title="Preview"
        description="This sample reflects the current subtitle appearance."
      >
        <div
          className="surface-panel-subtle relative overflow-hidden rounded-[1.3rem]"
          style={{ aspectRatio: "16 / 9", background: "linear-gradient(135deg, #0f0f1a, #1a1a3e)" }}
        >
          <div
            className="absolute inset-x-0 z-10 flex flex-col items-center gap-1"
            style={containerStyle}
          >
            <span
              className="inline-block rounded px-3 py-1 text-center leading-snug"
              style={{ ...cueStyle, whiteSpace: "pre-line" }}
            >
              Sample subtitle text
            </span>
            <span
              className="inline-block rounded px-3 py-1 text-center leading-snug"
              style={{ ...cueStyle, whiteSpace: "pre-line" }}
            >
              tuned for readability
            </span>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button onClick={handleSave} disabled={!hasUnsavedChanges || setDeviceSetting.isPending}>
            Save Appearance
          </Button>
          <Button
            variant="outline"
            onClick={discardLocalChanges}
            disabled={!hasUnsavedChanges || setDeviceSetting.isPending}
          >
            Discard Changes
          </Button>
          {hasDeviceOverride ? (
            <Button
              variant="ghost"
              onClick={handleUseFallback}
              disabled={deleteDeviceSetting.isPending}
            >
              <RotateCcw className="mr-2 h-4 w-4" />
              Reset Appearance
            </Button>
          ) : null}
        </div>
      </SettingsGroup>

      <SettingsGroup title="Text" description="Adjust the look and readability of subtitle text.">
        <SettingRow label="Font size">
          {({ id, descriptionId }) => (
            <Select
              value={settings.fontSize}
              onValueChange={(value) => update("fontSize", value as SubtitleAppearance["fontSize"])}
            >
              <SelectTrigger id={id} aria-describedby={descriptionId} className="w-full sm:w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {FONT_SIZE_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </SettingRow>

        <SettingRow label="Font family">
          {({ id, descriptionId }) => (
            <Select
              value={settings.fontFamily}
              onValueChange={(value) =>
                update("fontFamily", value as SubtitleAppearance["fontFamily"])
              }
            >
              <SelectTrigger id={id} aria-describedby={descriptionId} className="w-full sm:w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {FONT_FAMILY_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </SettingRow>

        <SettingRow label="Font color" labelForControl={false}>
          {({ labelId, descriptionId }) => (
            <ColorPalette
              colors={FONT_COLOR_PALETTE}
              selected={settings.fontColor}
              onChange={(hex) => update("fontColor", hex)}
              labelId={labelId}
              descriptionId={descriptionId}
            />
          )}
        </SettingRow>

        <SettingRow label="Text outline">
          {({ id, descriptionId }) => (
            <Switch
              id={id}
              aria-describedby={descriptionId}
              checked={settings.textOutline}
              onCheckedChange={(checked) => update("textOutline", checked)}
            />
          )}
        </SettingRow>

        <SettingRow
          label="Outline color"
          labelForControl={false}
          description="Only used when text outline is enabled."
        >
          {({ labelId, descriptionId }) => (
            <ColorPalette
              colors={FONT_COLOR_PALETTE}
              selected={settings.textOutlineColor}
              onChange={(hex) => update("textOutlineColor", hex)}
              disabled={!usesTextOutline}
              labelId={labelId}
              descriptionId={descriptionId}
            />
          )}
        </SettingRow>
      </SettingsGroup>

      <SettingsGroup
        title="Background & Position"
        description="Tune subtitle placement and contrast."
      >
        <SettingRow label="Background style">
          {({ id, descriptionId }) => (
            <Select
              value={settings.backgroundStyle}
              onValueChange={(value) =>
                update("backgroundStyle", value as SubtitleAppearance["backgroundStyle"])
              }
            >
              <SelectTrigger id={id} aria-describedby={descriptionId} className="w-full sm:w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {BACKGROUND_STYLE_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </SettingRow>

        <SettingRow label="Background opacity" description="Only used for boxed subtitles.">
          {({ descriptionId }) => (
            <div className="flex w-full max-w-[240px] items-center gap-3">
              <Slider
                aria-describedby={descriptionId}
                value={[settings.backgroundOpacity]}
                min={0}
                max={100}
                step={5}
                disabled={!isBoxStyle}
                onValueChange={(values) =>
                  update("backgroundOpacity", values[0] ?? settings.backgroundOpacity)
                }
              />
              <span className="text-muted-foreground min-w-10 text-right text-xs font-medium">
                {settings.backgroundOpacity}%
              </span>
            </div>
          )}
        </SettingRow>

        <SettingRow label="Background color" labelForControl={false}>
          {({ labelId, descriptionId }) => (
            <ColorPalette
              colors={BG_COLOR_PALETTE}
              selected={settings.backgroundColor}
              onChange={(hex) => update("backgroundColor", hex)}
              disabled={!isBoxStyle}
              labelId={labelId}
              descriptionId={descriptionId}
            />
          )}
        </SettingRow>

        <SettingRow label="Subtitle position">
          {({ id, descriptionId }) => (
            <Select
              value={settings.position}
              onValueChange={(value) => update("position", value as SubtitleAppearance["position"])}
            >
              <SelectTrigger id={id} aria-describedby={descriptionId} className="w-full sm:w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {POSITION_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </SettingRow>
      </SettingsGroup>
    </div>
  );
}
