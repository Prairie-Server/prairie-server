import { describe, expect, it } from "vitest";

import { buildCollectionPreviewRequest, previewFingerprint } from "./collectionPreviews";

describe("collection preview helpers", () => {
  it("builds a normalized preview request", () => {
    expect(
      buildCollectionPreviewRequest({
        library_ids: [2, 1],
        media_scope: "movie",
        match: "all",
        groups: [],
        sort: { field: "rating", order: "desc" },
      }),
    ).toEqual({
      query_definition: {
        library_ids: [2, 1],
        media_scope: "movie",
        match: "all",
        groups: [],
        sort: { field: "rating_imdb", order: "desc" },
        limit: undefined,
      },
      limit: 12,
    });
  });

  it("includes the scope in the preview cache fingerprint", () => {
    const request = buildCollectionPreviewRequest();

    expect(previewFingerprint("user", request)).not.toEqual(previewFingerprint("admin", request));
  });
});
