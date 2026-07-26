import type { SkippableStep } from "./setupStorage";

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

/** First incomplete step — the wizard frontier users advance into. */
export function deriveFrontierStep(
  accountComplete: boolean,
  profileComplete: boolean,
  stepDone: Record<SkippableStep, boolean>,
): WizardStepId {
  if (!accountComplete) return "account";
  if (!profileComplete) return "profile";
  if (!stepDone.server) return "server";
  if (!stepDone.integrations) return "integrations";
  if (!stepDone.downloads) return "downloads";
  if (!stepDone.recommendations) return "recommendations";
  if (!stepDone.library) return "library";
  return "nodes";
}

/**
 * Resolve the visible step when reviewing a previous step.
 * Review is clamped so it can never point past the frontier.
 */
export function resolveCurrentStep(
  frontierStep: WizardStepId,
  reviewStep: WizardStepId | null,
): WizardStepId {
  if (!reviewStep) return frontierStep;
  if (wizardStepIndex(reviewStep) <= wizardStepIndex(frontierStep)) return reviewStep;
  return frontierStep;
}

/** Previous step id when going back, or null at the first step. */
export function previousWizardStep(currentStep: WizardStepId): WizardStepId | null {
  const idx = wizardStepIndex(currentStep);
  if (idx <= 0) return null;
  return WIZARD_STEP_ORDER[idx - 1] ?? null;
}

/**
 * Next review target when continuing from a past step.
 * Returns null to clear review (return to the frontier).
 */
export function nextReviewStep(
  currentStep: WizardStepId,
  frontierStep: WizardStepId,
): WizardStepId | null {
  const idx = wizardStepIndex(currentStep);
  const frontierIdx = wizardStepIndex(frontierStep);
  if (idx >= frontierIdx) return null;
  const next = WIZARD_STEP_ORDER[idx + 1];
  if (!next || next === frontierStep) return null;
  return next;
}
