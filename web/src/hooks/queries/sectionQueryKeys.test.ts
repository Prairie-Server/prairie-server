import { describe, expect, it } from "vitest";
import { sectionKeys } from "./keys";

describe("sectionKeys", () => {
  it("builds stable progressive section keys", () => {
    expect(sectionKeys.homeLayout()).toEqual(["sections", "home", "layout"]);
    expect(sectionKeys.homeItems("hero")).toEqual(["sections", "home", "items", "hero"]);
    expect(sectionKeys.libraryLayout(42)).toEqual(["sections", "library", 42, "layout"]);
    expect(sectionKeys.libraryItemsRoot(42)).toEqual(["sections", "library", 42, "items"]);
    expect(sectionKeys.libraryItems(42, "hero")).toEqual([
      "sections",
      "library",
      42,
      "items",
      "hero",
    ]);
  });
});
