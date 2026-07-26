import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { RecommendationsStep } from "./RecommendationsStep";

const useSettingsFormMock = vi.fn();
const useWizardContextMock = vi.fn();
const useCheckAdminSettingsConnectionMock = vi.fn();

vi.mock("@/hooks/useSettingsForm", () => ({
  useSettingsForm: (...args: unknown[]) => useSettingsFormMock(...args),
}));

vi.mock("../WizardContext", () => ({
  useWizardContext: (...args: unknown[]) => useWizardContextMock(...args),
}));

vi.mock("@/hooks/queries/admin/settings", () => ({
  useCheckAdminSettingsConnection: (...args: unknown[]) =>
    useCheckAdminSettingsConnectionMock(...args),
}));

describe("RecommendationsStep", () => {
  it("renders a connection check action when recommendations are enabled", () => {
    useWizardContextMock.mockReturnValue({ markDone: vi.fn(), canGoBack: false, goBack: vi.fn() });
    useCheckAdminSettingsConnectionMock.mockReturnValue({
      isPending: false,
      mutateAsync: vi.fn(),
    });
    useSettingsFormMock.mockReturnValue({
      isLoading: false,
      getValue: (key: string) => {
        if (key === "recommendations.enabled") return "true";
        return "";
      },
      setValue: vi.fn(),
      dirtyCount: 0,
      save: vi.fn(),
      discard: vi.fn(),
      isSaving: false,
      restartRequired: false,
      sensitiveConfigured: [],
      buildConnectionCheckRequest: vi.fn(),
    });

    const markup = renderToStaticMarkup(<RecommendationsStep />);

    expect(markup).toContain("Check Connection");
  });
});
