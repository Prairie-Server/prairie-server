import { PrairieBrand } from "@/components/PrairieBrand";
import { useServerBranding } from "@/hooks/useServerBranding";
import { cn } from "@/lib/utils";

interface AuthBrandHeroProps {
  /** Optional override for the supporting sentence under the brand. */
  subtitle?: string;
  className?: string;
}

/**
 * First-viewport brand composition for auth surfaces.
 * App mark + server name (avoids stacking the wordmark and a duplicate title).
 */
export function AuthBrandHero({ subtitle, className }: AuthBrandHeroProps) {
  const { serverName, loginSubtitle } = useServerBranding();
  const supporting = subtitle ?? loginSubtitle;

  return (
    <header className={cn("auth-brand", className)}>
      <PrairieBrand
        variant="mark"
        className="auth-brand-mark h-14 w-14 sm:h-16 sm:w-16"
        imageClassName="rounded-xl"
      />
      <div className="auth-brand-copy space-y-2.5">
        <p className="auth-brand-title">{serverName}</p>
        {supporting ? <p className="auth-brand-subtitle">{supporting}</p> : null}
      </div>
    </header>
  );
}
