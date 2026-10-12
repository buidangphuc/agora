import { render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  beaconsOf,
  beginBeaconCapture,
  drain,
  endBeaconCapture,
  listEntries,
  listIds,
  viewListing,
} from "@/test/listBeacons";

const loadSearch = vi.hoisted(() => vi.fn());
const loadCategories = vi.hoisted(() => vi.fn());

vi.mock("./data", () => ({
  loadSearch,
  loadCategories,
  searchKey: (s: unknown) => JSON.stringify(s),
}));
vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  notFound: vi.fn(),
  useRouter: () => ({ push: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/lib/gateway/shops", () => ({
  batchGetShopNames: async () => new Map<string, string>(),
  shopLabel: (id: string, name?: string) => name || `Shop #${id.slice(0, 6)}`,
}));
vi.mock("@/features/engagement/FavoriteButton", () => ({
  FavoriteButton: () => null,
}));

import { SearchResultsBlock } from "./SearchBlocks";
import { parseSearchParams } from "./url";

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  fetchMock = beginBeaconCapture();
  loadCategories.mockResolvedValue([]);
  loadSearch.mockResolvedValue({
    ok: true,
    result: {
      items: [viewListing("a"), viewListing("b"), viewListing("c")],
      total: 3,
      page: 1,
      pageSize: 24,
      facets: {
        categories: [],
        priceRanges: [],
        ratings: [],
        sellers: [],
        tags: [],
        skus: [],
      },
    },
  });
});
afterEach(endBeaconCapture);

describe("SearchResultsBlock item list id", () => {
  it("[Fan-out rows share a group id] every beacon and view_item_list item carries item list id search_results", async () => {
    render(await SearchResultsBlock({ state: parseSearchParams({ q: "ao" }) }));
    await drain();
    expect(listIds(fetchMock)).toEqual({
      beacon: ["search_results"],
      layer: ["search_results"],
    });
  });

  it("[One request per rendered list] results are announced once: a single view_item_list entry and one beacon per listing (no SearchImpressions + grid double fire)", async () => {
    render(await SearchResultsBlock({ state: parseSearchParams({ q: "ao" }) }));
    await drain();
    expect(listEntries()).toHaveLength(1);
    expect(beaconsOf(fetchMock)).toHaveLength(3);
  });

  it("[Fan-out rows share a group id] search impressions keep the query: beacons carry query and the view_item_list has search_term", async () => {
    render(await SearchResultsBlock({ state: parseSearchParams({ q: "ao" }) }));
    await drain();
    const beacons = beaconsOf(fetchMock);
    expect(beacons).toHaveLength(3);
    for (const b of beacons) expect(b.query).toBe("ao");
    const layer = (
      window as unknown as {
        dataLayer: { event?: string; search_term?: string }[];
      }
    ).dataLayer;
    const entry = layer.filter((e) => e.event === "view_item_list");
    expect(entry).toHaveLength(1);
    expect(entry[0].search_term).toBe("ao");
  });
});
