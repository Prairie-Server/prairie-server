import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mocks = vi.hoisted(() => ({
  useQuery: vi.fn(),
  useOptionalAuth: vi.fn(),
  api: vi.fn(),
}));

vi.mock("@tanstack/react-query", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-query")>(
    "@tanstack/react-query",
  );
  return {
    ...actual,
    useQuery: (...args: unknown[]) => mocks.useQuery(...args),
  };
});

vi.mock("@/hooks/useAuth", () => ({
  useOptionalAuth: () => mocks.useOptionalAuth(),
}));

vi.mock("@/api/client", () => ({
  api: (...args: unknown[]) => mocks.api(...args),
}));

import { useProfiles } from "./profiles";
import { profileKeys } from "./keys";

function render(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return renderToStaticMarkup(
    <QueryClientProvider client={client}>{node}</QueryClientProvider>,
  );
}

function CallUseProfiles() {
  useProfiles();
  return null;
}

describe("useProfiles", () => {
  beforeEach(() => {
    mocks.useQuery.mockReset();
    mocks.useOptionalAuth.mockReset();
    mocks.api.mockReset();
    mocks.useQuery.mockReturnValue({ data: undefined, isLoading: false });
  });

  it("does not fetch profiles before sign-in", () => {
    mocks.useOptionalAuth.mockReturnValue({ user: null });
    render(<CallUseProfiles />);

    const options = mocks.useQuery.mock.calls[0]![0] as {
      enabled: boolean;
      queryKey: readonly unknown[];
    };
    expect(options.queryKey).toEqual(profileKeys.list());
    expect(options.enabled).toBe(false);
  });

  it("fetches profiles once a user is signed in", () => {
    mocks.useOptionalAuth.mockReturnValue({ user: { id: 1 } });
    render(<CallUseProfiles />);

    const options = mocks.useQuery.mock.calls[0]![0] as { enabled: boolean };
    expect(options.enabled).toBe(true);
  });

  it("stays disabled outside AuthProvider", () => {
    mocks.useOptionalAuth.mockReturnValue(null);
    render(<CallUseProfiles />);

    const options = mocks.useQuery.mock.calls[0]![0] as { enabled: boolean };
    expect(options.enabled).toBe(false);
  });
});
