import { createElement, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  api: vi.fn(),
  setValue: vi.fn(),
}));

vi.mock("@/api/client", () => ({
  api: mocks.api,
}));

vi.mock("@/hooks/queries/settingValues", () => ({
  useEffectiveSettings: () => ({ data: undefined, isLoading: false }),
  useSetSettingValue: () => ({ mutate: mocks.setValue }),
}));

vi.mock("@/utils/storage", () => ({
  storage: {
    KEYS: { PROFILE_ID: "profile_id" },
    get: () => null,
  },
}));

import { useOverlayPrefs } from "./useOverlayPrefs";
import { useUpdateServerSettings } from "./queries/admin/settings";

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(
      QueryClientProvider,
      { client: queryClient },
      children,
    );
  };
}

describe("useOverlayPrefs", () => {
  beforeEach(() => {
    mocks.api.mockReset();
    mocks.setValue.mockReset();
  });

  afterEach(cleanup);

  it("reads the server-wide overlay configuration without bypassing the shared query cache", async () => {
    mocks.api.mockResolvedValue({ enabled: true });
    const { result } = renderHook(() => useOverlayPrefs(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(mocks.api).toHaveBeenCalledWith("/settings/overlay-config");
    expect(result.current.enabled).toBe(true);
    expect(result.current).not.toHaveProperty("watchedIndicatorStyle");
  });

  it("refreshes the shared overlay configuration immediately after an admin save", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
    });
    const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");
    mocks.api.mockResolvedValue({});
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);
    const { result } = renderHook(() => useUpdateServerSettings(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({ "defaults.card_overlays": "{}" });
    });

    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: ["settings", "overlay-config"],
    });
  });
});
