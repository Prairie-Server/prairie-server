import { useCallback, useEffect, useState } from "react";
import type { FormEvent } from "react";
import { Link } from "react-router";
import { MonitorSmartphone } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/api/client";
import type { DeviceLoginLookupResponse } from "@/api/types";
import { SettingsGroup } from "@/components/settings/SettingsGroup";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";

function normalizeCode(value: string) {
  const clean = value
    .toUpperCase()
    .replace(/[^A-Z0-9]/g, "")
    .slice(0, 8);
  if (clean.length <= 4) {
    return clean;
  }
  return `${clean.slice(0, 4)}-${clean.slice(4)}`;
}

export default function QuickConnectSettings() {
  const [codeInput, setCodeInput] = useState("");
  const [activeCode, setActiveCode] = useState("");
  const [details, setDetails] = useState<DeviceLoginLookupResponse | null>(null);
  const [loadingDetails, setLoadingDetails] = useState(false);
  const [acting, setActing] = useState(false);

  useDocumentTitle("Quick Connect Settings");

  const loadDetails = useCallback(async (code: string) => {
    if (!code) {
      setDetails(null);
      return;
    }

    setLoadingDetails(true);
    try {
      const params = new URLSearchParams({ code });
      const result = await api<DeviceLoginLookupResponse>(`/auth/device?${params.toString()}`);
      setDetails(result);
    } catch (error) {
      setDetails(null);
      toast.error(error instanceof Error ? error.message : "Device request not found");
    } finally {
      setLoadingDetails(false);
    }
  }, []);

  useEffect(() => {
    if (!activeCode) {
      setDetails(null);
      return;
    }
    void loadDetails(activeCode);
  }, [activeCode, loadDetails]);

  function handleCodeSubmit(e: FormEvent) {
    e.preventDefault();
    const normalized = normalizeCode(codeInput);
    if (!normalized || normalized.replace(/-/g, "").length < 8) {
      toast.error("Enter the full 8-character code from the other device");
      return;
    }
    setActiveCode(normalized);
  }

  async function handleDecision(action: "approve" | "deny") {
    if (!activeCode) {
      return;
    }
    setActing(true);
    try {
      await api(`/auth/device/${action}`, {
        method: "POST",
        body: JSON.stringify({ code: activeCode }),
      });
      await loadDetails(activeCode);
      toast.success(action === "approve" ? "Device approved" : "Device denied");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : `Failed to ${action} request`);
    } finally {
      setActing(false);
    }
  }

  function reset() {
    setCodeInput("");
    setActiveCode("");
    setDetails(null);
  }

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <h2 className="text-2xl font-semibold tracking-tight">Quick Connect</h2>
        <p className="text-muted-foreground text-sm leading-relaxed">
          Approve sign-in for another Prairie device without typing a password there. Enter the code
          shown on the TV, Roku, Smart TV, or web login screen.
        </p>
      </div>

      <SettingsGroup
        title="Enter device code"
        description="Codes look like ABCD-EFGH and expire after a few minutes."
      >
        {!activeCode ? (
          <form onSubmit={handleCodeSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="quick-connect-code">Code from the other screen</Label>
              <Input
                id="quick-connect-code"
                value={codeInput}
                onChange={(e) => setCodeInput(normalizeCode(e.target.value))}
                autoCapitalize="characters"
                autoComplete="off"
                autoCorrect="off"
                placeholder="ABCD-EFGH"
                className="font-mono tracking-wider"
              />
            </div>
            <Button type="submit">Continue</Button>
          </form>
        ) : loadingDetails ? (
          <p className="text-muted-foreground text-sm">Looking up device request…</p>
        ) : !details ? (
          <div className="space-y-4">
            <p className="text-sm">That sign-in request could not be found or has expired.</p>
            <Button type="button" variant="outline" onClick={reset}>
              Enter another code
            </Button>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="border-border/60 bg-background/40 flex gap-3 rounded-md border p-4">
              <div className="bg-primary/10 text-primary flex h-10 w-10 shrink-0 items-center justify-center rounded-full">
                <MonitorSmartphone className="h-5 w-5" />
              </div>
              <div className="min-w-0 space-y-1">
                <div className="text-lg font-semibold">{details.device_name || "This device"}</div>
                {details.device_platform ? (
                  <div className="text-muted-foreground text-sm">{details.device_platform}</div>
                ) : null}
                {details.ip_address_hint ? (
                  <div className="text-muted-foreground text-sm">{details.ip_address_hint}</div>
                ) : null}
                {details.match_code ? (
                  <div className="mt-3">
                    <div className="text-muted-foreground text-xs tracking-[0.12em] uppercase">
                      Match code
                    </div>
                    <div className="text-lg font-semibold">{details.match_code}</div>
                    <p className="text-muted-foreground mt-1 text-xs">
                      Confirm this phrase matches the other screen before approving.
                    </p>
                  </div>
                ) : null}
              </div>
            </div>

            {details.status === "pending" ? (
              <div className="flex flex-wrap gap-3">
                <Button disabled={acting} onClick={() => void handleDecision("approve")}>
                  {acting ? "Approving…" : "Approve sign-in"}
                </Button>
                <Button
                  variant="outline"
                  disabled={acting}
                  onClick={() => void handleDecision("deny")}
                >
                  Deny
                </Button>
              </div>
            ) : details.status === "approved" ? (
              <p className="text-sm">Approved. Finish sign-in on the other device.</p>
            ) : details.status === "consumed" ? (
              <p className="text-sm">This device is already signed in.</p>
            ) : details.status === "denied" ? (
              <p className="text-sm">This sign-in request was denied.</p>
            ) : (
              <p className="text-sm">This sign-in request has expired.</p>
            )}

            <Button type="button" variant="ghost" onClick={reset}>
              Enter another code
            </Button>
          </div>
        )}
      </SettingsGroup>

      <SettingsGroup
        title="How it works"
        description="Any Prairie client that shows a Quick Connect code can be approved here."
      >
        <ol className="text-muted-foreground list-decimal space-y-2 pl-5 text-sm leading-relaxed">
          <li>
            On the other device, choose Quick Connect (or Show QR code) on the sign-in screen.
          </li>
          <li>Enter the displayed code here, confirm the match phrase, and approve.</li>
          <li>The other device finishes signing in automatically.</li>
        </ol>
        <p className="text-muted-foreground text-sm">
          You can also approve from{" "}
          <Link className="text-foreground underline underline-offset-2" to="/activate">
            /activate
          </Link>{" "}
          or from the Prairie mobile app.
        </p>
      </SettingsGroup>
    </div>
  );
}
