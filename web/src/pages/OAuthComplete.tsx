import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { setAccessToken, setRefreshToken } from "@/api/client";
import { api } from "@/api/client";
import type { RefreshResponse, User } from "@/api/types";
import { useAuth } from "@/hooks/useAuth";
import { sanitizeAuthRedirect } from "@/lib/authRedirect";

type OAuthCompleteResponse = RefreshResponse & {
  next: string;
};

async function completeOAuthCode(code: string): Promise<OAuthCompleteResponse> {
  const res = await fetch("/api/v1/auth/oauth/complete", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code }),
  });
  if (!res.ok) {
    throw new Error("Sign-in response expired. Please try again.");
  }
  return (await res.json()) as OAuthCompleteResponse;
}

export default function OAuthComplete() {
  const navigate = useNavigate();
  const { completeLogin } = useAuth();
  const [error, setError] = useState<string | null>(() =>
    new URLSearchParams(window.location.search).get("code")
      ? null
      : "Sign-in response missing completion code. Please try again.",
  );

  useEffect(() => {
    const code = new URLSearchParams(window.location.search).get("code");
    if (!code) {
      return;
    }
    window.history.replaceState(null, "", window.location.pathname);

    let cancelled = false;
    void (async () => {
      try {
        const tokens = await completeOAuthCode(code);
        if (cancelled) return;
        const next = sanitizeAuthRedirect(tokens.next) || "/";
        setAccessToken(tokens.access_token);
        setRefreshToken(tokens.refresh_token);
        const user = await api<User>("/auth/me");
        if (cancelled) return;
        completeLogin({
          access_token: tokens.access_token,
          refresh_token: tokens.refresh_token,
          expires_in: tokens.expires_in,
          user,
        });
        void navigate(next, { replace: true });
      } catch (err) {
        if (!cancelled) {
          setAccessToken(null);
          setRefreshToken(null);
          setError(err instanceof Error ? err.message : "Failed to complete sign-in");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [completeLogin, navigate]);

  if (error) {
    return (
      <div className="auth-shell">
        <div className="border-destructive/30 bg-destructive/10 text-destructive max-w-md rounded-md border p-4 text-sm">
          {error}
        </div>
      </div>
    );
  }

  return (
    <div className="auth-shell">
      <div className="border-primary h-8 w-8 animate-spin rounded-full border-b-2" />
    </div>
  );
}
