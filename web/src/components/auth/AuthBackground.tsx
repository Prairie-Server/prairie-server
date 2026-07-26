import { useBranding } from "@/hooks/useBranding";

/**
 * Full-bleed custom login background with a readability scrim. Renders nothing
 * when no login background image is configured, so it is safe to drop into any
 * auth page. Must be the first child of an `.auth-shell` element. Renders a
 * `position: fixed` wrapper that sits above the shell's fixed gradient ::before
 * layer (both at z-index 0) and below the `.auth-card` (z-index 1).
 */
export function AuthBackground() {
  const { loginBgUrl } = useBranding();
  if (!loginBgUrl) return null;

  return (
    <div aria-hidden className="pointer-events-none fixed inset-0 z-0">
      <img src={loginBgUrl} alt="" className="h-full w-full object-cover" />
      <div className="bg-background/70 absolute inset-0" />
    </div>
  );
}
