// @vitest-environment jsdom

import { act } from "react";
import type { ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import Home from "./Home";

(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const mockUseHomeLayout = vi.fn();
const mockFetchHomeSectionItems = vi.fn();

vi.mock("@/hooks/queries/sections", () => ({
  useHomeLayout: (...args: unknown[]) => mockUseHomeLayout(...args),
  fetchHomeSectionItems: (...args: unknown[]) => mockFetchHomeSectionItems(...args),
}));

vi.mock("@/hooks/useDocumentTitle", () => ({
  useDocumentTitle: vi.fn(),
}));

vi.mock("@/hooks/useServerBranding", () => ({
  useServerBranding: () => ({ serverName: "Prairie", loginSubtitle: null }),
}));

vi.mock("@/components/PrairieBrand", () => ({
  PrairieBrand: () => <span data-kind="prairie-brand" />,
}));

vi.mock("react-router", () => ({
  Link: ({ children }: { children: ReactNode }) => <a>{children}</a>,
}));

vi.mock("@/components/TasteSeedBanner", () => ({
  default: () => <div data-kind="taste-seed" />,
}));

vi.mock("@/components/livetv/LiveTVOnNowRow", () => ({
  default: () => null,
}));

vi.mock("@/components/HeroBanner", () => ({
  default: () => <div data-kind="hero" />,
}));

vi.mock("@/components/SectionRow", () => ({
  default: () => <div data-kind="section-row" />,
}));

function layoutSection(overrides: {
  id: string;
  title?: string;
  featured?: boolean;
  item_limit?: number;
}) {
  return {
    id: overrides.id,
    title: overrides.title ?? overrides.id,
    section_type: "recently_added",
    featured: overrides.featured ?? false,
    item_limit: overrides.item_limit ?? 16,
    is_custom: false,
    customized: false,
    position: 0,
  };
}

describe("Home", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);

    mockFetchHomeSectionItems.mockReset();
    mockUseHomeLayout.mockReturnValue({
      data: { sections: [] },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
  });

  afterEach(async () => {
    await act(async () => {
      root.unmount();
    });
    container.remove();
  });

  async function renderHome(queryClient = new QueryClient()) {
    await act(async () => {
      root.render(
        <QueryClientProvider client={queryClient}>
          <Home />
        </QueryClientProvider>,
      );
      await Promise.resolve();
    });
    return queryClient;
  }

  it("does not invalidate cached home sections on mount", async () => {
    const invalidateQueries = vi.spyOn(QueryClient.prototype, "invalidateQueries");
    await renderHome();
    expect(invalidateQueries).not.toHaveBeenCalled();
    invalidateQueries.mockRestore();
  });

  it("shows a compact brand welcome only when home has no sections", async () => {
    await renderHome();
    expect(container.querySelector('[aria-label="Welcome"]')).toBeTruthy();
    expect(container.querySelector('[data-kind="hero"]')).toBeNull();
  });

  it("opens on carousel rows when sections exist without a featured hero", async () => {
    mockFetchHomeSectionItems.mockResolvedValue({
      section: {
        id: "recent",
        title: "Recently Added",
        section_type: "recently_added",
        featured: false,
        items: [{ content_id: "m1", title: "Movie", type: "movie" }],
        total_count: 1,
      },
    });
    mockUseHomeLayout.mockReturnValue({
      data: { sections: [layoutSection({ id: "recent", title: "Recently Added" })] },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    await renderHome();
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.querySelector('[aria-label="Welcome"]')).toBeNull();
    expect(container.querySelector('[data-kind="hero"]')).toBeNull();
  });
});
