import { beforeEach, describe, expect, it, vi } from "vitest";

const mockUseQuery = vi.fn();
const mockApi = vi.fn();

vi.mock("@tanstack/react-query", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-query")>(
    "@tanstack/react-query",
  );
  return {
    ...actual,
    useQuery: (...args: unknown[]) => mockUseQuery(...args),
    useMutation: () => ({
      mutate: vi.fn(),
      mutateAsync: vi.fn(),
      isPending: false,
    }),
    useQueryClient: () => ({ invalidateQueries: vi.fn() }),
  };
});

vi.mock("@/api/client", () => ({
  api: (...args: unknown[]) => mockApi(...args),
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn(), message: vi.fn() },
}));

import { useLiveTVGuide, useLiveTVRecordings } from "./useLiveTV";

describe("useLiveTV guide and recordings", () => {
  beforeEach(() => {
    mockUseQuery.mockReset();
    mockApi.mockReset();
    mockUseQuery.mockImplementation((options: unknown) => options);
    mockApi.mockResolvedValue({ programs: [], recordings: [] });
  });

  it("requests guide with channel and window params", async () => {
    useLiveTVGuide({
      channelIds: ["ch1", "ch2"],
      start: "2026-07-25T18:00:00Z",
      end: "2026-07-26T00:00:00Z",
    });
    const queryOptions = mockUseQuery.mock.calls[0]?.[0] as {
      queryFn: () => Promise<unknown>;
    };
    await queryOptions.queryFn();
    expect(mockApi).toHaveBeenCalledWith(
      "/livetv/guide?channels=ch1%2Cch2&start=2026-07-25T18%3A00%3A00Z&end=2026-07-26T00%3A00%3A00Z",
    );
  });

  it("requests recordings with optional status", async () => {
    useLiveTVRecordings("scheduled");
    const queryOptions = mockUseQuery.mock.calls[0]?.[0] as {
      queryFn: () => Promise<unknown>;
    };
    await queryOptions.queryFn();
    expect(mockApi).toHaveBeenCalledWith("/livetv/recordings?status=scheduled");
  });
});
