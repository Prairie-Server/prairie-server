import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { useWizardContext } from "./WizardContext";

import { ArrowLeft } from "lucide-react";
interface WizardActionsProps {
  children?: ReactNode;
  className?: string;
}

/**
 * Shared setup-wizard action row. Prepends Back when a previous step is reachable.
 */
export function WizardActions({
  children,
  className = "flex flex-wrap gap-3 pt-2",
}: WizardActionsProps) {
  const { canGoBack, goBack } = useWizardContext();

  if (!canGoBack && (children === undefined || children === null || children === false)) {
    return null;
  }

  return (
    <div className={className}>
      {canGoBack ? (
        <Button type="button" variant="ghost" onClick={goBack}>
          <ArrowLeft />
          Back
        </Button>
      ) : null}
      {children}
    </div>
  );
}
