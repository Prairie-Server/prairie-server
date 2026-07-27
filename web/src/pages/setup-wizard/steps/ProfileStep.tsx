import { useState } from "react";
import type { FormEvent } from "react";
import { api } from "@/api/client";
import type { CreateProfileRequest, Profile } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { toast } from "sonner";
import { useWizardContext } from "../WizardContext";
import { WizardActions } from "../WizardActions";

import { ArrowRight, Loader2, Plus } from "lucide-react";
export function ProfileStep() {
  const { selectProfile, refetchProfiles, profiles, goForward } = useWizardContext();
  const [profileName, setProfileName] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    try {
      const body: CreateProfileRequest = { name: profileName };
      const created = await api<Profile>("/profiles", {
        method: "POST",
        body: JSON.stringify(body),
      });
      selectProfile(created);
      refetchProfiles();
      toast.success("Profile created");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create profile");
    } finally {
      setSubmitting(false);
    }
  }

  // Reviewing a completed profile step when navigating back.
  if (profiles.length > 0) {
    const names = profiles.map((p) => p.name).join(", ");
    return (
      <div className="space-y-4">
        <p className="text-muted-foreground text-sm leading-relaxed">
          Profile{profiles.length === 1 ? "" : "s"}{" "}
          <span className="text-foreground font-medium">{names}</span>{" "}
          {profiles.length === 1 ? "is" : "are"} ready. Continue to review the next step.
        </p>
        <WizardActions>
          <Button type="button" onClick={goForward}>
            <ArrowRight />
            Continue
          </Button>
        </WizardActions>
      </div>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="setup-profile-name" className="text-xs">
          Name
        </Label>
        <Input
          id="setup-profile-name"
          value={profileName}
          onChange={(e) => setProfileName(e.target.value)}
          placeholder="Alex"
          autoComplete="nickname"
          required
        />
      </div>
      <WizardActions className="flex flex-wrap gap-3 pt-3">
        <Button type="submit" disabled={submitting}>
          {submitting ? <Loader2 className="animate-spin" /> : <Plus />}
          {submitting ? "Creating..." : "Create profile"}
        </Button>
      </WizardActions>
    </form>
  );
}
