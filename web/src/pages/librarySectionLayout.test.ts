import { describe, expect, it } from "vitest";

import type { ResolvedSection } from "@/api/types";

import { splitLibrarySections } from "./librarySectionLayout";

function makeSection(overrides: Partial<ResolvedSection>): ResolvedSection {
  return {
    id: overrides.id ?? "section-1",
    section_type: overrides.section_type ?? "collection",
    title: overrides.title ?? "Section",
    featured: overrides.featured ?? false,
    item_limit: overrides.item_limit ?? 20,
    total_count: overrides.total_count ?? 1,
    is_custom: overrides.is_custom ?? false,
    customized: overrides.customized ?? false,
    items: overrides.items ?? [
      {
        content_id: "item-1",
        type: "series",
        title: "Item",
        genres: [],
        status: "matched",
        year: 2025,
        rating_imdb: null,
        overview: "",
        poster_url: "",
        poster_thumbhash: "",
        backdrop_url: "",
        backdrop_thumbhash: "",
        logo_url: "",
      },
    ],
  };
}

describe("splitLibrarySections", () => {
  it("lifts the first featured section into the hero slot", () => {
    const hero = makeSection({ id: "hero", featured: true, title: "Hero" });
    const row = makeSection({ id: "row", title: "Row" });

    const result = splitLibrarySections([hero, row]);

    expect(result.hero?.id).toBe("hero");
    expect(result.rows.map((section) => section.id)).toEqual(["row"]);
  });

  it("ignores featured sections with no items", () => {
    const emptyFeatured = makeSection({
      id: "empty",
      featured: true,
      items: [],
      total_count: 0,
    });
    const row = makeSection({ id: "row", title: "Row" });

    const result = splitLibrarySections([emptyFeatured, row]);

    expect(result.hero).toBeNull();
    expect(result.rows.map((section) => section.id)).toEqual(["row"]);
  });
});
