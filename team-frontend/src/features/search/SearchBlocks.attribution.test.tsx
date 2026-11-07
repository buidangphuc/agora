import { render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  beaconsOf,
  beginBeaconCapture,
  drain,
  endBeaconCapture,
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

const emptyFacets = {
  categories: [],
  priceRanges: [],
  ratings: [],
  sellers: [],
  tags: [],
  skus: [],
};

function loaded(extra: Record<string, unknown>) {
  return {
    ok: true,
    result: {
      items: [viewListing("a"), viewListing("b")],
      total: 2,
      page: 1,
      pageSize: 24,
      facets: emptyFacets,
      ...extra,
    },
  };
}

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  fetchMock = beginBeaconCapture();
  loadCategories.mockResolvedValue([]);
});
afterEach(endBeaconCapture);

describe("SearchResultsBlock attribution", () => {
  it("[Search page beacons carry the response identity] every impression carries impressionId, modelVersion and placementId search_results", async () => {
    loadSearch.mockResolvedValue(
      loaded({ requestId: "r".repeat(32), modelVersion: "hybrid@v1" }),
    );
    render(await SearchResultsBlock({ state: parseSearchParams({ q: "ao" }) }));
    await drain();
    const beacons = beaconsOf(fetchMock);
    expect(beacons).toHaveLength(2);
    for (const b of beacons) {
      expect(b.impressionId).toBe("r".repeat(32));
      expect(b.modelVersion).toBe("hybrid@v1");
      expect(b.placementId).toBe("search_results");
    }
  });

  it("[Search page beacons carry the response identity] an empty request id omits impressionId but keeps modelVersion and placement", async () => {
    loadSearch.mockResolvedValue(
      loaded({ requestId: "", modelVersion: "lexical@v1" }),
    );
    render(await SearchResultsBlock({ state: parseSearchParams({ q: "ao" }) }));
    await drain();
    const beacons = beaconsOf(fetchMock);
    expect(beacons).toHaveLength(2);
    for (const b of beacons) {
      expect(b).not.toHaveProperty("impressionId");
      expect(b.modelVersion).toBe("lexical@v1");
      expect(b.placementId).toBe("search_results");
    }
  });

  it("[A gateway response without an id still renders] a response with no attribution fields renders the listings and beacons without impressionId", async () => {
    loadSearch.mockResolvedValue(loaded({}));
    const { container } = render(
      await SearchResultsBlock({ state: parseSearchParams({ q: "ao" }) }),
    );
    await drain();
    expect(container.textContent).toContain("SP a");
    expect(container.textContent).toContain("SP b");
    const beacons = beaconsOf(fetchMock);
    expect(beacons).toHaveLength(2);
    for (const b of beacons) {
      expect(b).not.toHaveProperty("impressionId");
      expect(b.placementId).toBe("search_results");
    }
  });

  it("[Search page beacons carry the response identity] a failed search sends no beacons", async () => {
    loadSearch.mockResolvedValue({ ok: false });
    render(await SearchResultsBlock({ state: parseSearchParams({ q: "ao" }) }));
    await drain();
    expect(beaconsOf(fetchMock)).toHaveLength(0);
  });
});
