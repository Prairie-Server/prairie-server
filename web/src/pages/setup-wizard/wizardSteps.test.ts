import { describe, expect, it } from "vitest";
import { WIZARD_STEP_ORDER, wizardStepIndex } from "./wizardSteps";

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
});
