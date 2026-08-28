import { beforeEach, describe, expect, it, vi } from "vitest";

import { createEmptyQueryDefinition } from "@/api/types";

const mocks = vi.hoisted(() => ({
  api: vi.fn(),
}));

vi.mock("@/api/client", () => ({
  api: mocks.api,
}));

import { fetchCatalogFilters, fetchCatalogPage } from "./catalog";

describe("catalog query helpers", () => {
  beforeEach(() => {
    mocks.api.mockReset();
    mocks.api.mockResolvedValue({ items: [], total: 0, has_more: false });
  });

  it("fetches query-source metadata filters from the catalog filters endpoint", async () => {
    await fetchCatalogFilters({
      source: "query",
      query_definition: createEmptyQueryDefinition(),
    });

    expect(mocks.api).toHaveBeenCalledWith(
      "/catalog/filters?source=query",
      undefined,
    );
  });

  it("can request lightweight catalog filters for guided UIs", async () => {
    await fetchCatalogFilters(
      {
        source: "query",
        q: "house",
        query_definition: createEmptyQueryDefinition(),
      },
      undefined,
      { includeTechnical: false },
    );

    expect(mocks.api).toHaveBeenCalledWith(
      "/catalog/filters?source=query&q=house&include_technical=false",
      undefined,
    );
  });

  it("fetches browse pages from the catalog list endpoint", async () => {
    await fetchCatalogPage(
      {
        source: "query",
        library_id: 7,
        query_definition: createEmptyQueryDefinition(),
      },
      60,
      120,
    );

    expect(mocks.api).toHaveBeenCalledWith(
      "/catalog?source=query&library_id=7&sort=added_at&order=desc&limit=60&offset=120",
      undefined,
    );
  });

  it("can skip exact total counts for lightweight browse requests", async () => {
    await fetchCatalogPage(
      {
        source: "query",
        library_id: 7,
        query_definition: createEmptyQueryDefinition(),
      },
      60,
      0,
      undefined,
      false,
    );

    expect(mocks.api).toHaveBeenCalledWith(
      "/catalog?source=query&library_id=7&sort=added_at&order=desc&limit=60&offset=0&include_total=false",
      undefined,
    );
  });
});
