import { useWizardContext } from "./WizardContext";

export type { WizardStepId } from "./wizardSteps";

export interface StepDef {
  id: string;
  label: string;
  complete: boolean;
  active: boolean;
}

export function useWizardSteps() {
  const { user, profiles, stepDone, currentStep, frontierStep } =
    useWizardContext();

  const accountComplete = !!user;
  const profileComplete = profiles.length > 0;
  const libraryDone = stepDone.library;

  const steps: StepDef[] = [
    {
      id: "account",
      label: "Account",
      complete: accountComplete,
      active: currentStep === "account",
    },
    {
      id: "profile",
      label: "Profile",
      complete: profileComplete,
      active: currentStep === "profile",
    },
    {
      id: "server",
      label: "Server",
      complete: stepDone.server,
      active: currentStep === "server",
    },
    {
      id: "integrations",
      label: "Integrations",
      complete: stepDone.integrations,
      active: currentStep === "integrations",
    },
    {
      id: "downloads",
      label: "Downloads",
      complete: stepDone.downloads,
      active: currentStep === "downloads",
    },
    {
      id: "recommendations",
      label: "Recs",
      complete: stepDone.recommendations,
      active: currentStep === "recommendations",
    },
    {
      id: "library",
      label: "Library",
      complete: libraryDone,
      active: currentStep === "library",
    },
    {
      id: "nodes",
      label: "Finish",
      complete: false,
      active: currentStep === "nodes",
    },
  ];

  return { steps, currentStep, frontierStep };
}
