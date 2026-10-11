import { render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  beaconsOf,
  beginBeaconCapture,
  drain,
  endBeaconCapture,
  listIds,
  viewListing,
} from "@/test/listBeacons";

const loadFeed = vi.hoisted(() => vi.fn());
const getRecentlyViewed = vi.hoisted(() => vi.fn());
const getListing = vi.hoisted(() => vi.fn());

vi.mock("./data", () => ({ loadFeed }));
vi.mock("@/features/engagement/FavoriteButton", () => ({
  FavoriteButton: () => null,
}));
vi.mock("@/lib/gateway/engagement", () => ({ getRecentlyViewed }));
vi.mock("@/lib/gateway/listings", () => ({ getListing }));

import { FeedBlock } from "./FeedBlock";
import { RecentlyViewedRow } from "./RecentlyViewedRow";

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  fetchMock = beginBeaconCapture();
});
afterEach(endBeaconCapture);

describe("home list item list ids", () => {
  it("[Fan-out rows share a group id] FeedBlock beacons and view_item_list carry item list id home_feed", async () => {
    loadFeed.mockResolvedValue({
      items: [viewListing("a"), viewListing("b"), viewListing("c")],
      failed: false,
    });
    render(await FeedBlock());
    await drain();
    expect(beaconsOf(fetchMock)).toHaveLength(3);
    expect(listIds(fetchMock)).toEqual({
      beacon: ["home_feed"],
      layer: ["home_feed"],
    });
  });

  it("[Fan-out rows share a group id] RecentlyViewedRow beacons and view_item_list carry item list id recently_viewed", async () => {
    getRecentlyViewed.mockResolvedValue(["a", "b"]);
    getListing.mockImplementation(async (id: string) => viewListing(id));
    render(await RecentlyViewedRow({}));
    await drain();
    expect(beaconsOf(fetchMock)).toHaveLength(2);
    expect(listIds(fetchMock)).toEqual({
      beacon: ["recently_viewed"],
      layer: ["recently_viewed"],
    });
  });
});
