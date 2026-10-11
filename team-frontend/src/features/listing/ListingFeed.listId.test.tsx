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

vi.mock("@/features/engagement/FavoriteButton", () => ({
  FavoriteButton: () => null,
}));

import { ListingFeed } from "./ListingFeed";

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  fetchMock = beginBeaconCapture();
});
afterEach(endBeaconCapture);

describe("ListingFeed item list id", () => {
  it("[Fan-out rows share a group id] beacons and view_item_list carry item list id listing_feed", async () => {
    render(
      <ListingFeed
        initial={[viewListing("a"), viewListing("b")]}
        initialCursor=""
        endpoint="/api/x"
      />,
    );
    await drain();
    expect(beaconsOf(fetchMock)).toHaveLength(2);
    expect(listIds(fetchMock)).toEqual({
      beacon: ["listing_feed"],
      layer: ["listing_feed"],
    });
  });
});
