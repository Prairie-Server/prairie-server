import { describe, expect, it } from "vitest";
import { createEmptySetupWizardFlags } from "./setupStorage";
import {
  WIZARD_STEP_ORDER,
  deriveFrontierStep,
  nextReviewStep,
  previousWizardStep,
  resolveCurrentStep,
  reviewStepAfterMarkDone,
  wizardStepIndex,
} from "./wizardSteps";

describe("wizardSteps", () => {
  it("keeps a stable 8-step order", () => {
    expect(WIZARD_STEP_ORDER).toEqual([
      "account",
      "profile",
      "server",
      "integrations",
      "downloads",
      "recommendations",
      "library",
      "nodes",
    ]);
  });

  it("indexes steps for back navigation", () => {
    expect(wizardStepIndex("account")).toBe(0);
    expect(wizardStepIndex("integrations")).toBe(3);
    expect(wizardStepIndex("nodes")).toBe(7);
  });

  describe("deriveFrontierStep", () => {
    it("starts at account until an admin user exists", () => {
      expect(
        deriveFrontierStep(false, false, createEmptySetupWizardFlags()),
      ).toBe("account");
    });

    it("moves to profile after account, then server after profile", () => {
      expect(
        deriveFrontierStep(true, false, createEmptySetupWizardFlags()),
      ).toBe("profile");
      expect(
        deriveFrontierStep(true, true, createEmptySetupWizardFlags()),
      ).toBe("server");
    });

    it("walks skippable steps in order until finish", () => {
      const done = createEmptySetupWizardFlags();
      done.server = true;
      expect(deriveFrontierStep(true, true, done)).toBe("integrations");
      done.integrations = true;
      expect(deriveFrontierStep(true, true, done)).toBe("downloads");
      done.downloads = true;
      expect(deriveFrontierStep(true, true, done)).toBe("recommendations");
      done.recommendations = true;
      expect(deriveFrontierStep(true, true, done)).toBe("library");
      done.library = true;
      expect(deriveFrontierStep(true, true, done)).toBe("nodes");
    });
  });

  describe("resolveCurrentStep", () => {
    it("uses the frontier when not reviewing", () => {
      expect(resolveCurrentStep("integrations", null)).toBe("integrations");
    });

    it("allows reviewing an earlier completed step", () => {
      expect(resolveCurrentStep("integrations", "server")).toBe("server");
    });

    it("clamps review that somehow points past the frontier", () => {
      expect(resolveCurrentStep("server", "downloads")).toBe("server");
    });
  });

  describe("previousWizardStep / nextReviewStep", () => {
    it("returns null when already on the first step", () => {
      expect(previousWizardStep("account")).toBeNull();
    });

    it("returns the prior step for back navigation", () => {
      expect(previousWizardStep("integrations")).toBe("server");
      expect(previousWizardStep("profile")).toBe("account");
    });

    it("advances review one step at a time toward the frontier", () => {
      expect(nextReviewStep("account", "integrations")).toBe("profile");
      expect(nextReviewStep("profile", "integrations")).toBe("server");
      expect(nextReviewStep("server", "integrations")).toBeNull();
    });

    it("clears review when already at the frontier", () => {
      expect(nextReviewStep("integrations", "integrations")).toBeNull();
    });
  });

  describe("reviewStepAfterMarkDone", () => {
    it("advances one step when continuing from an earlier reviewed step", () => {
      const done = createEmptySetupWizardFlags();
      done.server = true;
      done.integrations = true;
      done.downloads = true;
      done.recommendations = true;
      // Frontier is library (step 7 of 8). Reviewing downloads (step 5) should go to recommendations.
      expect(
        reviewStepAfterMarkDone("downloads", "downloads", true, true, done),
      ).toBe("recommendations");
    });

    it("returns to the frontier when continuing from the step immediately before it", () => {
      const done = createEmptySetupWizardFlags();
      done.server = true;
      done.integrations = true;
      done.downloads = true;
      done.recommendations = true;
      expect(
        reviewStepAfterMarkDone(
          "recommendations",
          "recommendations",
          true,
          true,
          done,
        ),
      ).toBeNull();
    });

    it("clears review when continuing on the frontier so the next frontier is shown", () => {
      const done = createEmptySetupWizardFlags();
      done.server = true;
      done.integrations = true;
      expect(
        reviewStepAfterMarkDone("downloads", "downloads", true, true, done),
      ).toBeNull();
      expect(deriveFrontierStep(true, true, { ...done, downloads: true })).toBe(
        "recommendations",
      );
    });
  });
});
