import { describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  api: vi.fn(),
}));

vi.mock("@/api/client", () => ({
  api: mocks.api,
}));

import {
  fetchHomeSectionItems,
  fetchLibrarySectionItems,
  useLibraryLayout,
  normalizeProfileSectionOverridesResponse,
} from "./sections";

describe("sections query helpers", () => {
  it("fetches home section items from the home section items endpoint", async () => {
    const options = { cache: "no-store" } satisfies RequestInit;
    mocks.api.mockResolvedValue({ section: { items: [] } });

    await fetchHomeSectionItems("section-1", options);

    expect(mocks.api).toHaveBeenCalledWith(
      "/home/sections/section-1/items",
      options,
    );
  });

  it("fetches library section items from the library section items endpoint", async () => {
    const options = { cache: "no-store" } satisfies RequestInit;
    mocks.api.mockResolvedValue({ section: { items: [] } });

    await fetchLibrarySectionItems(1, "section-1", options);

    expect(mocks.api).toHaveBeenCalledWith(
      "/library/1/sections/section-1/items",
      options,
    );
  });

  it("exports the library layout hook", () => {
    expect(useLibraryLayout).toBeTypeOf("function");
  });

  it("normalizes backend-shaped raw section overrides into frontend override fields", () => {
    const response = normalizeProfileSectionOverridesResponse({
      overrides: [
        {
          ID: "override-1",
          SectionID: "admin-1",
          Position: 2,
          Hidden: true,
          Removed: true,
          SectionType: "recently_added",
          Title: "Recently Added",
          Featured: true,
          ItemLimit: 12,
          Config: '{"media_scope":"movie"}',
        },
      ],
    });

    expect(response).toEqual({
      overrides: [
        {
          id: "override-1",
          section_id: "admin-1",
          position: 2,
          hidden: true,
          removed: true,
          section_type: "recently_added",
          title: "Recently Added",
          featured: true,
          item_limit: 12,
          config: { media_scope: "movie" },
        },
      ],
    });
  });
});
