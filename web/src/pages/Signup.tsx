import { useState } from "react";
import type { FormEvent } from "react";
import { Link, Navigate, useNavigate, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { Profile, SignupStatusResponse } from "@/api/types";
import { getBootstrapProfile, useAuth } from "@/hooks/useAuth";
import { Button } from "@/components/ui/button";
import { PasswordInput } from "@/components/PasswordInput";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { AuthBackground } from "@/components/auth/AuthBackground";
import { AuthBrandHero } from "@/components/auth/AuthBrandHero";
import { sanitizeAuthRedirect } from "@/lib/authRedirect";
import { toast } from "sonner";

export default function Signup() {
  const [searchParams] = useSearchParams();
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [inviteCode, setInviteCode] = useState(searchParams.get("code") ?? "");
  const [submitting, setSubmitting] = useState(false);
  const { signup, profile, selectProfile, user, loading } = useAuth();
  const navigate = useNavigate();
  const redirectTarget = sanitizeAuthRedirect(searchParams.get("redirect"));

  const statusQuery = useQuery({
    queryKey: ["auth", "signup-status"],
    queryFn: () => api<SignupStatusResponse>("/auth/signup"),
  });

  if (loading || statusQuery.isPending) {
    return (
      <div className="auth-shell">
        <div className="border-primary h-8 w-8 animate-spin rounded-full border-b-2" />
      </div>
    );
  }

  if (user) {
    return <Navigate to={redirectTarget || (profile ? "/" : "/profiles")} replace />;
  }

  if (statusQuery.data && !statusQuery.data.enabled) {
    return (
      <div className="auth-shell">
        <AuthBackground />
        <div className="auth-viewport">
          <AuthBrandHero subtitle="Public signups are currently closed." />
        </div>
      </div>
    );
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (password !== confirmPassword) {
      toast.error("Passwords do not match");
      return;
    }
    setSubmitting(true);
    try {
      await signup(username, email, password, inviteCode);
      if (redirectTarget) {
        navigate(redirectTarget, { replace: true });
        return;
      }
      try {
        const profileList = await api<{ profiles: Profile[] }>("/profiles");
        const soleProfile = getBootstrapProfile(profileList.profiles ?? []);
        if (soleProfile) {
          selectProfile(soleProfile);
          navigate("/");
          return;
        }
      } catch {
        navigate("/profiles");
        return;
      }
      navigate("/profiles");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Signup failed");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="auth-shell">
      <AuthBackground />
      <div className="auth-viewport">
        <AuthBrandHero subtitle="Create a new account to get started." />
        <Card className="auth-card border-0 bg-transparent shadow-none">
          <CardHeader className="sr-only">
            <CardTitle>Create account</CardTitle>
            <CardDescription>Create a new account to get started.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-5 p-0">
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="signup-username">Username</Label>
                <Input
                  id="signup-username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  autoComplete="username"
                  autoFocus
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="signup-email">Email</Label>
                <Input
                  id="signup-email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  autoComplete="email"
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="signup-password">Password</Label>
                <p className="text-muted-foreground text-xs">At least 8 characters</p>
                <PasswordInput
                  id="signup-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="new-password"
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="signup-confirm-password">Confirm password</Label>
                <PasswordInput
                  id="signup-confirm-password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  autoComplete="new-password"
                  required
                />
                {confirmPassword && password !== confirmPassword && (
                  <p className="text-destructive text-xs">Passwords do not match</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="signup-invite-code">Invite code</Label>
                <Input
                  id="signup-invite-code"
                  value={inviteCode}
                  onChange={(e) => setInviteCode(e.target.value)}
                  placeholder="Enter your invite code"
                  required
                />
              </div>
              <Button type="submit" className="w-full" disabled={submitting}>
                {submitting ? "Creating account..." : "Create account"}
              </Button>
            </form>
            <p className="text-muted-foreground mt-4 text-center text-sm">
              Already have an account?{" "}
              <Link
                to={
                  redirectTarget
                    ? `/login?redirect=${encodeURIComponent(redirectTarget)}`
                    : "/login"
                }
                className="text-foreground underline hover:no-underline"
              >
                Sign in
              </Link>
            </p>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
