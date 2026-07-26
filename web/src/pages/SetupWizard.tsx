import { Navigate } from "react-router";
import { useAuth } from "@/hooks/useAuth";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import { AuthBackground } from "@/components/auth/AuthBackground";
import { AuthBrandHero } from "@/components/auth/AuthBrandHero";
import { WizardProvider, useWizardContext } from "./setup-wizard/WizardContext";
import { useWizardSteps } from "./setup-wizard/useWizardSteps";
import { StepIndicator } from "./setup-wizard/StepIndicator";
import { AccountStep } from "./setup-wizard/steps/AccountStep";
import { ProfileStep } from "./setup-wizard/steps/ProfileStep";
import { LibraryStep } from "./setup-wizard/steps/LibraryStep";
import { ServerStorageStep } from "./setup-wizard/steps/ServerStorageStep";
import { IntegrationsStep } from "./setup-wizard/steps/IntegrationsStep";
import { DownloadsStep } from "./setup-wizard/steps/DownloadsStep";
import { RecommendationsStep } from "./setup-wizard/steps/RecommendationsStep";
import { NodesFinishStep } from "./setup-wizard/steps/NodesFinishStep";
import type { WizardStepId } from "./setup-wizard/useWizardSteps";

const STEP_TITLES: Record<WizardStepId, string> = {
  account: "Create your account",
  profile: "Add a profile",
  server: "Server & storage",
  integrations: "Integrations",
  downloads: "Downloads",
  recommendations: "Recommendations",
  library: "Add a library",
  nodes: "You're all set",
};

const STEP_DESCRIPTIONS: Record<WizardStepId, string> = {
  account: "This will be the admin account for managing your server.",
  profile: "Profiles let different people track their own watch history and preferences.",
  server: "Configure core infrastructure. All fields are optional and can be changed later.",
  integrations: "Configure subtitle providers for automatic subtitle downloading.",
  downloads: "Allow users to download media files for offline viewing.",
  recommendations: "AI-powered recommendations using embeddings. Requires pgvector.",
  library: "Point Prairie at your media files. You can add more libraries later.",
  nodes: "Prairie is ready. Start exploring or fine-tune in admin settings.",
};

function WizardContent() {
  const { user, profiles, librariesLoading, profilesLoading } = useWizardContext();
  const { steps, currentStep } = useWizardSteps();
  const isAdmin = user?.role === "admin";
  const profileComplete = profiles.length > 0;

  if (profilesLoading || (user && profileComplete && isAdmin && librariesLoading)) {
    return (
      <div className="auth-shell items-start py-10 sm:py-14">
        <AuthBackground />
        <div className="auth-setup-panel relative z-1 w-full max-w-2xl p-1 pb-8 sm:p-2 sm:pb-10">
          <AuthBrandHero
            className="mb-8"
            subtitle="Getting your server ready — hang tight while we load setup."
          />
          <div className="space-y-3">
            <div className="flex gap-1">
              {Array.from({ length: 8 }).map((_, i) => (
                <div key={i} className="bg-foreground/[0.06] h-1 flex-1 rounded-full" />
              ))}
            </div>
            <div className="bg-foreground/[0.06] h-3 w-32 animate-pulse rounded" />
          </div>
          <div className="bg-foreground/[0.06] mt-8 h-8 w-48 animate-pulse rounded" />
          <div className="bg-foreground/[0.06] mt-3 h-4 w-72 animate-pulse rounded" />
        </div>
      </div>
    );
  }

  return (
    <div className="auth-shell items-start py-10 sm:py-14">
      <AuthBackground />
      <div className="auth-setup-panel relative z-1 w-full max-w-2xl p-1 pb-8 sm:p-2 sm:pb-10">
        <AuthBrandHero className="mb-8" subtitle="First-run setup for your Prairie server." />
        <div className="mb-10">
          <StepIndicator steps={steps} />
          <h1 className="font-display text-foreground mt-6 text-[1.7rem] leading-tight font-semibold tracking-[-0.03em] sm:text-3xl">
            {STEP_TITLES[currentStep]}
          </h1>
          <p className="text-muted-foreground mt-2 text-sm leading-relaxed">
            {STEP_DESCRIPTIONS[currentStep]}
          </p>
        </div>

        <div key={currentStep} className="animate-[fade-in_0.25s_ease-out]">
          {currentStep === "account" && <AccountStep />}
          {currentStep === "profile" && <ProfileStep />}
          {currentStep === "library" && <LibraryStep />}
          {currentStep === "server" && <ServerStorageStep />}
          {currentStep === "integrations" && <IntegrationsStep />}
          {currentStep === "downloads" && <DownloadsStep />}
          {currentStep === "recommendations" && <RecommendationsStep />}
          {currentStep === "nodes" && <NodesFinishStep />}
        </div>
      </div>
    </div>
  );
}

export default function SetupWizard() {
  const { user, loading, setupLoading, setupRequired } = useAuth();
  useDocumentTitle("Setup");

  if (loading || setupLoading) {
    return (
      <main className="auth-shell">
        <AuthBackground />
        <div className="text-muted-foreground relative z-1 text-sm">Loading setup…</div>
      </main>
    );
  }

  if (!setupRequired && !user) {
    return <Navigate to="/login" replace />;
  }

  if (user && user.role !== "admin") {
    return <Navigate to="/profiles" replace />;
  }

  return (
    <WizardProvider>
      <WizardContent />
    </WizardProvider>
  );
}
