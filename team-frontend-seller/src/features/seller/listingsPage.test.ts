import { beforeEach, describe, expect, it, vi } from "vitest";

import { ListingStatus } from "@/generated/platform/listing/v1/listing_pb.js";
import { listMyListings } from "@/lib/gateway/listings";
import * as listingsPage from "./listingsPage";

vi.mock("@/lib/gateway/listings", () => ({ listMyListings: vi.fn() }));

const page = {
  items: [{ id: "l1", title: "Quần jean" }],
  nextCursor: "more-should-not-be-followed",
  total: 2000,
};

beforeEach(() => vi.clearAllMocks());

describe("searchMyListings", () => {
  it("Picker opens with the first options: empty query is one PUBLISHED call of 20 without a cursor", async () => {
    vi.mocked(listMyListings).mockResolvedValue(page as never);
    const res = await listingsPage.searchMyListings("");
    expect(listMyListings).toHaveBeenCalledTimes(1);
    expect(listMyListings).toHaveBeenCalledWith({
      statuses: [ListingStatus.PUBLISHED],
      query: "",
      pageSize: 20,
    });
    expect(res.items[0].id).toBe("l1");
  });

  it("Typing narrows the options: the typed text is sent as the server query, no pages are walked", async () => {
    vi.mocked(listMyListings).mockResolvedValue(page as never);
    await listingsPage.searchMyListings("quần");
    expect(listMyListings).toHaveBeenCalledTimes(1);
    expect(listMyListings).toHaveBeenCalledWith({
      statuses: [ListingStatus.PUBLISHED],
      query: "quần",
      pageSize: 20,
    });
  });

  it("Search fails: a gateway error propagates to the caller", async () => {
    vi.mocked(listMyListings).mockRejectedValue(new Error("down"));
    await expect(listingsPage.searchMyListings("x")).rejects.toThrow("down");
  });
});

describe("listingsPage module surface", () => {
  it("Picker opens with the first options: cursor-walking helpers are gone", () => {
    expect("getListingsPage" in listingsPage).toBe(false);
    expect("getAllListings" in listingsPage).toBe(false);
  });
});
