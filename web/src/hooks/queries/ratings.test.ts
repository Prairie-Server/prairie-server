import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import type { ItemDetail } from "@/api/types";
import { fetchCatalogItemDetail } from "./catalogRead";
import { catalogKeys } from "./keys";
import { refreshCommunityRating } from "./ratings";

vi.mock("./catalogRead", () => ({ fetchCatalogItemDetail: vi.fn() }));

const key = catalogKeys.itemDetail("movie:c");

function seeded() {
  const client = new QueryClient();
  client.setQueryData(key, {
    content_id: "movie:c",
    user_rating: 4,
    community_rating: { average: 3.5, count: 4 },
  } as unknown as ItemDetail);
  return client;
}

function serverReturns(communityRating: ItemDetail["community_rating"]) {
  return {
    content_id: "movie:c",
    user_rating: 2,
    community_rating: communityRating,
  } as unknown as ItemDetail;
}

describe("refreshCommunityRating", () => {
  it("patches only the community rating into cached detail", async () => {
    const client = seeded();
    vi.mocked(fetchCatalogItemDetail).mockResolvedValueOnce(
      serverReturns({ average: 3.8, count: 5 }),
    );
    await refreshCommunityRating(client, "movie:c");
    const detail = client.getQueryData<ItemDetail>(key)!;
    expect(detail.community_rating).toEqual({ average: 3.8, count: 5 });
    expect(detail.user_rating).toBe(4);
  });

  it("clears the figure when the item drops below the display threshold", async () => {
    const client = seeded();
    vi.mocked(fetchCatalogItemDetail).mockResolvedValueOnce(serverReturns(undefined));
    await refreshCommunityRating(client, "movie:c");
    expect(client.getQueryData<ItemDetail>(key)!.community_rating).toBeNull();
  });

  it("ignores an older refresh that resolves after a newer one", async () => {
    const client = seeded();
    let resolveOld!: (detail: ItemDetail) => void;
    vi.mocked(fetchCatalogItemDetail)
      .mockReturnValueOnce(new Promise((resolve) => (resolveOld = resolve)))
      .mockResolvedValueOnce(serverReturns({ average: 4.2, count: 6 }));
    const older = refreshCommunityRating(client, "movie:c");
    await refreshCommunityRating(client, "movie:c");
    resolveOld(serverReturns({ average: 1, count: 3 }));
    await older;
    expect(client.getQueryData<ItemDetail>(key)!.community_rating).toEqual({
      average: 4.2,
      count: 6,
    });
  });
});
