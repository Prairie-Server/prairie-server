import { useState, useMemo } from "react";
import type { FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import { toast } from "sonner";
import { useSettingsForm } from "@/hooks/useSettingsForm";
import { useWizardContext } from "../WizardContext";
import { WizardActions } from "../WizardActions";

import { Loader2, Save, SkipForward } from "lucide-react";
const KEYS = [
  "download.enabled",
  "download.server_bandwidth_mbps",
  "download.user_bandwidth_mbps",
  "download.max_concurrent_per_user",
];

export function DownloadsStep() {
  const { markDone } = useWizardContext();
  const form = useSettingsForm({ keys: useMemo(() => KEYS, []) });
  const [submitting, setSubmitting] = useState(false);

  const enabled = form.getValue("download.enabled") === "true";

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (form.dirtyCount === 0) {
      markDone("downloads");
      return;
    }
    setSubmitting(true);
    try {
      await form.save();
      markDone("downloads");
      toast.success("Download settings saved");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save download settings");
    } finally {
      setSubmitting(false);
    }
  }

  function handleSkip() {
    markDone("downloads");
  }

  if (form.isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-14 w-full rounded-xl" />
        <Skeleton className="h-32 w-full rounded-xl" />
      </div>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {/* Enable toggle */}
      <div className="border-foreground/[0.07] bg-foreground/[0.03] flex items-center justify-between rounded-xl border px-4 py-3.5">
        <div>
          <Label htmlFor="download-enabled" className="text-sm font-medium">
            Enable downloads
          </Label>
          <p className="text-muted-foreground/70 mt-0.5 text-xs">Let users save files locally</p>
        </div>
        <Switch
          id="download-enabled"
          checked={enabled}
          onCheckedChange={(v) => form.setValue("download.enabled", v ? "true" : "false")}
        />
      </div>

      {enabled && (
        <div className="border-foreground/[0.07] bg-foreground/[0.03] animate-[fade-in_0.15s_ease-out] space-y-3 rounded-xl border p-4">
          <div>
            <p className="text-muted-foreground text-[11px] font-semibold tracking-[0.1em] uppercase">
              Bandwidth limits
            </p>
            <p className="text-muted-foreground/70 mt-0.5 text-xs">
              How much bandwidth can downloads consume? Does not affect streaming.
            </p>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="dl-server-bw" className="text-xs">
                Total download bandwidth
              </Label>
              <div className="flex items-center gap-2">
                <Input
                  id="dl-server-bw"
                  type="number"
                  value={form.getValue("download.server_bandwidth_mbps")}
                  onChange={(e) => form.setValue("download.server_bandwidth_mbps", e.target.value)}
                  className="w-24"
                  autoComplete="off"
                />
                <span className="text-muted-foreground text-xs">Mbps</span>
              </div>
              <p className="text-muted-foreground/60 text-[11px]">
                Shared across all users. 0 = unlimited.
              </p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="dl-user-bw" className="text-xs">
                Per-user download bandwidth
              </Label>
              <div className="flex items-center gap-2">
                <Input
                  id="dl-user-bw"
                  type="number"
                  value={form.getValue("download.user_bandwidth_mbps")}
                  onChange={(e) => form.setValue("download.user_bandwidth_mbps", e.target.value)}
                  className="w-24"
                  autoComplete="off"
                />
                <span className="text-muted-foreground text-xs">Mbps</span>
              </div>
              <p className="text-muted-foreground/60 text-[11px]">0 = unlimited</p>
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="dl-concurrent" className="text-xs">
              Max concurrent per user
            </Label>
            <Input
              id="dl-concurrent"
              type="number"
              value={form.getValue("download.max_concurrent_per_user")}
              onChange={(e) => form.setValue("download.max_concurrent_per_user", e.target.value)}
              className="w-24"
              autoComplete="off"
            />
            <p className="text-muted-foreground/60 text-[11px]">0 = unlimited</p>
          </div>
        </div>
      )}

      <WizardActions>
        <Button type="submit" disabled={submitting || form.isSaving}>
          {submitting || form.isSaving ? <Loader2 className="animate-spin" /> : <Save />}
          {submitting || form.isSaving ? "Saving..." : "Save & continue"}
        </Button>
        <Button
          type="button"
          variant="ghost"
          onClick={handleSkip}
          disabled={submitting || form.isSaving}
        >
          <SkipForward />
          Skip
        </Button>
      </WizardActions>
    </form>
  );
}
