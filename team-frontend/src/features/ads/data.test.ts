import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ViewListing } from "@/lib/gateway/listings";

const listSponsoredSlotSet = vi.hoisted(() => vi.fn());
const getListing = vi.hoisted(() => vi.fn());

vi.mock("@/lib/gateway/promotion", () => ({ listSponsoredSlotSet }));
vi.mock("@/lib/gateway/listings", () => ({ getListing }));

import { loadSponsored } from "./data";

function listing(id: string, status = "published"): ViewListing {
  return {
    id,
    title: `Listing ${id}`,
    description: "",
    price: 1000,
    currency: "VND",
    status,
    sellerId: "s",
    imageKeys: [],
    categoryId: "c",
    stock: 1,
    variants: [],
  };
}

function slot(n: number) {
  return {
    listingId: `l${n}`,
    campaignId: `c${n}`,
    impressionToken: `tok-${n}`,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  getListing.mockImplementation(async (id: string) => listing(id));
});

afterEach(() => vi.useRealTimers());

describe("loadSponsored", () => {
  it("requests and renders at most two slots", async () => {
    listSponsoredSlotSet.mockResolvedValue([
      slot(1),
      slot(2),
      slot(3),
      slot(4),
    ]);

    const out = await loadSponsored("search", "cat-1");

    expect(listSponsoredSlotSet).toHaveBeenCalledWith(
      expect.objectContaining({
        placement: "search",
        categoryId: "cat-1",
        limit: 2,
      }),
    );
    expect(out).toHaveLength(2);
    expect(out.map((s) => s.listing.id)).toEqual(["l1", "l2"]);
    expect(out.map((s) => s.impressionToken)).toEqual(["tok-1", "tok-2"]);
  });

  it("drops slots whose listing is missing or unpublished", async () => {
    listSponsoredSlotSet.mockResolvedValue([slot(1), slot(2), slot(3)]);
    getListing.mockImplementation(async (id: string) => {
      if (id === "l1") return null;
      if (id === "l2") return listing(id, "draft");
      return listing(id);
    });

    const out = await loadSponsored("home", "");

    expect(out.map((s) => s.listing.id)).toEqual(["l3"]);
    expect(listSponsoredSlotSet).toHaveBeenCalledWith(
      expect.objectContaining({ placement: "home", categoryId: "" }),
    );
  });

  it("sponsored failure returns no slots", async () => {
    listSponsoredSlotSet.mockRejectedValue(new Error("unavailable"));

    await expect(loadSponsored("search", "c")).resolves.toEqual([]);
  });

  it("sponsored timeout returns no slots after 150 ms", async () => {
    vi.useFakeTimers();
    listSponsoredSlotSet.mockReturnValue(new Promise(() => {}));

    const pending = loadSponsored("search", "c");
    await vi.advanceTimersByTimeAsync(150);

    await expect(pending).resolves.toEqual([]);
  });

  it("slow listing lookup is cut off at 150 ms", async () => {
    vi.useFakeTimers();
    listSponsoredSlotSet.mockResolvedValue([slot(1)]);
    getListing.mockReturnValue(new Promise(() => {}));

    const pending = loadSponsored("home", "");
    await vi.advanceTimersByTimeAsync(150);

    await expect(pending).resolves.toEqual([]);
  });
});
