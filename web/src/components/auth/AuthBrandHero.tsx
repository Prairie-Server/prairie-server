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
 * Brand mark is hero-level; server name uses display type as secondary signal.
 */
export function AuthBrandHero({ subtitle, className }: AuthBrandHeroProps) {
  const { serverName, loginSubtitle } = useServerBranding();
  const supporting = subtitle ?? loginSubtitle;

  return (
    <header className={cn("auth-brand", className)}>
      <PrairieBrand className="auth-brand-mark h-14 w-[148px] sm:h-16 sm:w-[168px]" />
      <div className="auth-brand-copy space-y-2">
        <p className="auth-brand-title">{serverName}</p>
        {supporting ? <p className="auth-brand-subtitle">{supporting}</p> : null}
      </div>
    </header>
  );
}
