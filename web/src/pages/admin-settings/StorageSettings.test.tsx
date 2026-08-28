import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import StorageSettings from "./StorageSettings";

const useSettingsFormMock = vi.fn();
const useCheckAdminSettingsConnectionMock = vi.fn();

vi.mock("@/hooks/useSettingsForm", () => ({
  useSettingsForm: (...args: unknown[]) => useSettingsFormMock(...args),
}));

vi.mock("@/hooks/queries/admin/settings", () => ({
  useCheckAdminSettingsConnection: (...args: unknown[]) =>
    useCheckAdminSettingsConnectionMock(...args),
}));

describe("StorageSettings", () => {
  it("shows public and private storage sections with connection checks", () => {
    useCheckAdminSettingsConnectionMock.mockReturnValue({
      isPending: false,
      mutateAsync: vi.fn(),
    });
    useSettingsFormMock.mockReturnValue({
      isLoading: false,
      getValue: (key: string) => {
        if (key === "s3.public_url_auth") return "presigned";
        return "";
      },
      setValue: vi.fn(),
      dirtyCount: 0,
      save: vi.fn(),
      discard: vi.fn(),
      isSaving: false,
      restartRequired: false,
      sensitiveConfigured: [],
      sensitiveStatusReady: true,
      sensitiveStatusError: false,
      buildConnectionCheckRequest: vi.fn(),
      isDirty: () => false,
    });

    const markup = renderToStaticMarkup(<StorageSettings />);

    expect(markup).toContain("Public Assets");
    expect(markup).toContain("Private Internal");
    expect(markup).toContain("Check Connection");
    expect(markup).not.toContain("Storage location change");
  });

  it("warns about the artwork cache when a public storage identity field is edited", () => {
    useCheckAdminSettingsConnectionMock.mockReturnValue({
      isPending: false,
      mutateAsync: vi.fn(),
    });
    useSettingsFormMock.mockReturnValue({
      isLoading: false,
      getValue: (key: string) => {
        if (key === "s3.public_url_auth") return "presigned";
        return "";
      },
      setValue: vi.fn(),
      dirtyCount: 1,
      save: vi.fn(),
      discard: vi.fn(),
      isSaving: false,
      restartRequired: false,
      sensitiveConfigured: [],
      sensitiveStatusReady: true,
      sensitiveStatusError: false,
      buildConnectionCheckRequest: vi.fn(),
      isDirty: (key: string) => key === "s3.public_bucket",
    });

    const markup = renderToStaticMarkup(<StorageSettings />);

    expect(markup).toContain("Storage location change");
    expect(markup).toContain("will not change artwork cache records");
    expect(markup).toContain("manually run Reconcile Artwork Cache");
    expect(markup).toContain("manual Backfill Metadata Images");
    expect(markup).toContain("new or changed metadata");
    expect(markup).not.toContain("automatically re-caches anything missing");
  });

  it("requires an explicit action before replacing a configured S3 credential", async () => {
    const resetValue = vi.fn();
    const setValue = vi.fn();
    let resolveSave: (() => void) | undefined;
    const save = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveSave = resolve;
        }),
    );
    const discard = vi.fn();
    useCheckAdminSettingsConnectionMock.mockReturnValue({
      isPending: false,
      mutateAsync: vi.fn(),
    });
    useSettingsFormMock.mockReturnValue({
      isLoading: false,
      getValue: (key: string) =>
        key === "s3.public_url_auth" ? "presigned" : "",
      setValue,
      resetValue,
      dirtyCount: 1,
      save,
      discard,
      isSaving: false,
      restartRequired: false,
      sensitiveConfigured: ["s3.public_access_key", "s3.public_secret_key"],
      sensitiveStatusReady: true,
      sensitiveStatusError: false,
      buildConnectionCheckRequest: vi.fn(),
      isDirty: () => false,
    });

    render(<StorageSettings />);

    expect(screen.queryByLabelText("Access Key")).not.toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", { name: "Replace Access Key" }),
    );
    expect(screen.getByLabelText("Access Key")).toHaveAttribute(
      "type",
      "password",
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Keep saved Access Key" }),
    );
    expect(resetValue).toHaveBeenCalledWith("s3.public_access_key");
    expect(screen.queryByLabelText("Access Key")).not.toBeInTheDocument();

    await userEvent.click(
      screen.getByRole("button", { name: "Replace Secret Key" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Save Changes" }));
    await waitFor(() => expect(save).toHaveBeenCalledOnce());
    setValue.mockClear();
    await userEvent.type(
      screen.getByLabelText("Secret Key"),
      "late replacement",
    );
    expect(setValue).not.toHaveBeenCalled();
    await act(async () => resolveSave?.());
    await waitFor(() =>
      expect(screen.queryByLabelText("Secret Key")).not.toBeInTheDocument(),
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Replace Access Key" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(discard).toHaveBeenCalledOnce();
    expect(screen.queryByLabelText("Access Key")).not.toBeInTheDocument();
  });

  it("keeps a credential replacement open when saving fails", async () => {
    const save = vi.fn().mockRejectedValue(new Error("save failed"));
    useCheckAdminSettingsConnectionMock.mockReturnValue({
      isPending: false,
      mutateAsync: vi.fn(),
    });
    useSettingsFormMock.mockReturnValue({
      isLoading: false,
      getValue: (key: string) =>
        key === "s3.public_url_auth" ? "presigned" : "",
      setValue: vi.fn(),
      resetValue: vi.fn(),
      dirtyCount: 1,
      save,
      discard: vi.fn(),
      isSaving: false,
      restartRequired: false,
      sensitiveConfigured: ["s3.public_access_key"],
      sensitiveStatusReady: true,
      sensitiveStatusError: false,
      buildConnectionCheckRequest: vi.fn(),
      isDirty: () => false,
    });

    render(<StorageSettings />);

    await userEvent.click(
      screen.getByRole("button", { name: "Replace Access Key" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Save Changes" }));

    await waitFor(() => expect(save).toHaveBeenCalledOnce());
    expect(screen.getByLabelText("Access Key")).toHaveAttribute(
      "type",
      "password",
    );
  });

  it("keeps credential inputs unmounted until protected status is available", () => {
    let sensitiveStatusReady = false;
    useCheckAdminSettingsConnectionMock.mockReturnValue({
      isPending: false,
      mutateAsync: vi.fn(),
    });
    useSettingsFormMock.mockImplementation(() => ({
      isLoading: false,
      getValue: (key: string) =>
        key === "s3.public_url_auth" ? "presigned" : "",
      setValue: vi.fn(),
      resetValue: vi.fn(),
      dirtyCount: 0,
      save: vi.fn(),
      discard: vi.fn(),
      isSaving: false,
      restartRequired: false,
      sensitiveConfigured: ["s3.public_access_key"],
      sensitiveStatusReady,
      sensitiveStatusError: false,
      buildConnectionCheckRequest: vi.fn(),
      isDirty: () => false,
    }));

    const { rerender } = render(<StorageSettings />);

    expect(
      screen.getByRole("status", { name: "Loading settings" }),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("Access Key")).not.toBeInTheDocument();

    sensitiveStatusReady = true;
    rerender(<StorageSettings />);

    expect(
      screen.getByRole("button", { name: "Replace Access Key" }),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("Access Key")).not.toBeInTheDocument();
  });

  it("fails closed when protected credential status cannot be loaded", () => {
    useCheckAdminSettingsConnectionMock.mockReturnValue({
      isPending: false,
      mutateAsync: vi.fn(),
    });
    useSettingsFormMock.mockReturnValue({
      isLoading: false,
      sensitiveConfigured: [],
      sensitiveStatusReady: false,
      sensitiveStatusError: true,
    });

    render(<StorageSettings />);

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Protected credential status is unavailable",
    );
    expect(screen.queryByLabelText("Access Key")).not.toBeInTheDocument();
  });
});
