import { describe, expect, it } from "vitest";

import type { DisplayQueryDefinition } from "@/api/types";
import {
  COLLECTION_MEDIA_FILTER_OPTIONS,
  COLLECTION_WATCH_FILTER_OPTIONS,
  collectionMediaFilterLabel,
  collectionMediaFilterOptionsFromPresets,
  collectionWatchFilterLabel,
  collectionWatchFilterOptionsFromPresets,
  displayFiltersToQueryDefinition,
  queryDefinitionToDisplayFilters,
} from "./collectionDisplayFilters";

describe("collection filter labels and presets", () => {
  it("labels known watch and media presets", () => {
    expect(collectionWatchFilterLabel("unwatched")).toBe("Unwatched");
    expect(collectionWatchFilterLabel("watched")).toBe("Watched");
    expect(collectionMediaFilterLabel("movie")).toBe("Movies");
    expect(collectionMediaFilterLabel("series")).toBe("Shows");
  });

  it("falls back to All for unknown values", () => {
    expect(collectionWatchFilterLabel("nope" as never)).toBe("All");
    expect(collectionMediaFilterLabel("nope" as never)).toBe("All");
  });

  it("returns full option lists when presets are undefined", () => {
    expect(collectionWatchFilterOptionsFromPresets(undefined)).toEqual(
      COLLECTION_WATCH_FILTER_OPTIONS,
    );
    expect(collectionMediaFilterOptionsFromPresets(undefined)).toEqual(
      COLLECTION_MEDIA_FILTER_OPTIONS,
    );
  });

  it("filters option lists to allowed presets", () => {
    expect(
      collectionWatchFilterOptionsFromPresets(["all", "unwatched"]).map((o) => o.value),
    ).toEqual(["all", "unwatched"]);
    expect(collectionMediaFilterOptionsFromPresets(["movie"]).map((o) => o.value)).toEqual([
      "movie",
    ]);
  });
});

describe("displayFiltersToQueryDefinition", () => {
  it("returns undefined when both presets are all", () => {
    expect(displayFiltersToQueryDefinition("all", "all")).toBeUndefined();
  });

  it("builds a single AND group with a watched=true rule for the watched preset", () => {
    expect(displayFiltersToQueryDefinition("watched", "all")).toEqual({
      match: "all",
      groups: [{ match: "all", rules: [{ field: "watched", op: "is", value: true }] }],
    });
  });

  it("builds a watched=false rule for the unwatched preset", () => {
    expect(displayFiltersToQueryDefinition("unwatched", "all")).toEqual({
      match: "all",
      groups: [{ match: "all", rules: [{ field: "watched", op: "is", value: false }] }],
    });
  });

  it("builds a type rule for the media preset", () => {
    expect(displayFiltersToQueryDefinition("all", "movie")).toEqual({
      match: "all",
      groups: [{ match: "all", rules: [{ field: "type", op: "is", value: "movie" }] }],
    });
    expect(displayFiltersToQueryDefinition("all", "series")).toEqual({
      match: "all",
      groups: [{ match: "all", rules: [{ field: "type", op: "is", value: "series" }] }],
    });
  });

  it("places both rules in one group when both presets are set", () => {
    expect(displayFiltersToQueryDefinition("unwatched", "series")).toEqual({
      match: "all",
      groups: [
        {
          match: "all",
          rules: [
            { field: "watched", op: "is", value: false },
            { field: "type", op: "is", value: "series" },
          ],
        },
      ],
    });
  });

  it("omits library_ids / media_scope / sort / limit (filter-only fragment)", () => {
    const fragment = displayFiltersToQueryDefinition("watched", "movie")!;
    expect(fragment).not.toHaveProperty("library_ids");
    expect(fragment).not.toHaveProperty("media_scope");
    expect(fragment).not.toHaveProperty("sort");
    expect(fragment).not.toHaveProperty("limit");
  });
});

describe("queryDefinitionToDisplayFilters", () => {
  it("defaults to all/all for undefined and null fragments", () => {
    expect(queryDefinitionToDisplayFilters(undefined)).toEqual({ watch: "all", media: "all" });
    expect(queryDefinitionToDisplayFilters(null)).toEqual({ watch: "all", media: "all" });
  });

  it("defaults to all/all for a fragment with no recognized rules", () => {
    const def: DisplayQueryDefinition = {
      match: "all",
      groups: [{ match: "all", rules: [{ field: "genre", op: "is", value: "horror" }] }],
    };
    expect(queryDefinitionToDisplayFilters(def)).toEqual({ watch: "all", media: "all" });
  });

  it("reads watched=true / watched=false back to presets", () => {
    const watched: DisplayQueryDefinition = {
      match: "all",
      groups: [{ match: "all", rules: [{ field: "watched", op: "is", value: true }] }],
    };
    expect(queryDefinitionToDisplayFilters(watched).watch).toBe("watched");

    const unwatched: DisplayQueryDefinition = {
      match: "all",
      groups: [{ match: "all", rules: [{ field: "watched", op: "is", value: false }] }],
    };
    expect(queryDefinitionToDisplayFilters(unwatched).watch).toBe("unwatched");
  });

  it("reads rules in arbitrary order and across multiple groups", () => {
    const def: DisplayQueryDefinition = {
      match: "all",
      groups: [
        {
          match: "all",
          rules: [
            { field: "type", op: "is", value: "series" },
            { field: "genre", op: "is", value: "drama" },
            { field: "watched", op: "is", value: false },
          ],
        },
      ],
    };
    expect(queryDefinitionToDisplayFilters(def)).toEqual({ watch: "unwatched", media: "series" });
  });

  it("round-trips both presets through the fragment", () => {
    const fragment = displayFiltersToQueryDefinition("watched", "movie");
    expect(queryDefinitionToDisplayFilters(fragment)).toEqual({
      watch: "watched",
      media: "movie",
    });
  });

  it("tolerates a group with a missing rules array", () => {
    expect(queryDefinitionToDisplayFilters({ groups: [{}] })).toEqual({
      watch: "all",
      media: "all",
    });
  });
});
