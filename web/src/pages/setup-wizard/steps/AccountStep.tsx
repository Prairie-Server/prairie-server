import { useState } from "react";
import type { FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PasswordInput } from "@/components/PasswordInput";
import { toast } from "sonner";
import { useWizardContext } from "../WizardContext";
import { WizardActions } from "../WizardActions";

export function AccountStep() {
  const { setupInitialUser, user, goForward } = useWizardContext();
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (password !== confirmPassword) {
      toast.error("Passwords do not match");
      return;
    }

    setSubmitting(true);
    try {
      await setupInitialUser(username, email, password);
      toast.success("Admin account created");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create admin account");
    } finally {
      setSubmitting(false);
    }
  }

  // Reviewing a completed account step when navigating back.
  if (user) {
    return (
      <div className="space-y-4">
        <p className="text-muted-foreground text-sm leading-relaxed">
          Admin account <span className="text-foreground font-medium">{user.username}</span> is
          ready. Continue to review the next step.
        </p>
        <WizardActions>
          <Button type="button" onClick={goForward}>
            Continue
          </Button>
        </WizardActions>
      </div>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="setup-email" className="text-xs">
          Email
        </Label>
        <Input
          id="setup-email"
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          autoComplete="email"
          required
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="setup-username" className="text-xs">
          Username
        </Label>
        <Input
          id="setup-username"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoComplete="username"
          required
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="setup-password" className="text-xs">
          Password
        </Label>
        <PasswordInput
          id="setup-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="new-password"
          required
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="setup-confirm-password" className="text-xs">
          Confirm password
        </Label>
        <PasswordInput
          id="setup-confirm-password"
          value={confirmPassword}
          onChange={(e) => setConfirmPassword(e.target.value)}
          autoComplete="new-password"
          required
        />
      </div>
      <WizardActions className="flex flex-wrap gap-3 pt-3">
        <Button type="submit" disabled={submitting}>
          {submitting ? "Creating..." : "Create account"}
        </Button>
      </WizardActions>
    </form>
  );
}
