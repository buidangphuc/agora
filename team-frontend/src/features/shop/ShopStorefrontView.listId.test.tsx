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
vi.mock("@/features/chat/ChatWithSellerButton", () => ({
  ChatWithSellerButton: () => null,
}));
vi.mock("@/features/engagement/FollowSellerButton", () => ({
  FollowSellerButton: () => null,
}));

import { ShopStorefrontView } from "./ShopStorefrontView";

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  fetchMock = beginBeaconCapture();
});
afterEach(endBeaconCapture);

describe("ShopStorefrontView item list id", () => {
  it("[Fan-out rows share a group id] beacons and view_item_list carry item list id shop_storefront", async () => {
    render(
      <ShopStorefrontView
        sellerId="s1"
        listings={[viewListing("a"), viewListing("b")]}
        loggedIn={false}
        summary={{
          sellerId: "s1",
          averageRating: 0,
          reviewCount: 0,
          breakdown: { star1: 0, star2: 0, star3: 0, star4: 0, star5: 0 },
        }}
      />,
    );
    await drain();
    expect(beaconsOf(fetchMock)).toHaveLength(2);
    expect(listIds(fetchMock)).toEqual({
      beacon: ["shop_storefront"],
      layer: ["shop_storefront"],
    });
  });
});
