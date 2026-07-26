export type WizardStepId =
  | "account"
  | "profile"
  | "server"
  | "integrations"
  | "downloads"
  | "recommendations"
  | "library"
  | "nodes";

export const WIZARD_STEP_ORDER: readonly WizardStepId[] = [
  "account",
  "profile",
  "server",
  "integrations",
  "downloads",
  "recommendations",
  "library",
  "nodes",
] as const;

export function wizardStepIndex(step: WizardStepId): number {
  return WIZARD_STEP_ORDER.indexOf(step);
}
