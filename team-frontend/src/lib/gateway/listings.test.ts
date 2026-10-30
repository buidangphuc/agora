import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ListingStatus } from "@/generated/platform/listing/v1/listing_pb.js";

import { makeClients } from "./client.js";
import {
  getListing,
  getStorefront,
  listListings,
  suggest,
} from "./listings.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

function stub(
  clients: Record<string, Record<string, ReturnType<typeof vi.fn>>>,
) {
  vi.mocked(makeClients).mockReturnValue(clients as never);
}

const protoListing = {
  id: "l1",
  title: "Phone",
  description: "A phone",
  price: 5000000n,
  currency: "VND",
  status: ListingStatus.PUBLISHED,
  sellerId: "s1",
  imageKeys: ["k1"],
  categoryId: "c1",
  stock: 3,
  variants: [],
};

beforeEach(() => vi.clearAllMocks());

describe("listings gateway wrapper", () => {
  it("listListings maps request paging + status and view-maps results", async () => {
    const listListingsRpc = vi.fn().mockResolvedValue({
      listings: [protoListing],
      page: { nextCursor: "next", total: 42n },
    });
    stub({ listing: { listListings: listListingsRpc } });

    const page = await listListings({ status: "published", pageSize: 10 });
    expect(listListingsRpc).toHaveBeenCalledWith({
      status: "published",
      page: { cursor: "", pageSize: 10 },
    });
    expect(page.nextCursor).toBe("next");
    expect(page.total).toBe(42);
    expect(page.items[0]).toMatchObject({
      id: "l1",
      status: "published",
      price: 5000000,
      imageUrl: "http://localhost:9000/listing-images/k1",
    });
  });

  it("getListing returns null on a NotFound ConnectError", async () => {
    const getListingRpc = vi
      .fn()
      .mockRejectedValue(new ConnectError("missing", Code.NotFound));
    stub({ listing: { getListing: getListingRpc } });
    await expect(getListing("l1")).resolves.toBeNull();
  });

  it("getListing rethrows non-NotFound errors", async () => {
    const getListingRpc = vi
      .fn()
      .mockRejectedValue(new ConnectError("boom", Code.Internal));
    stub({ listing: { getListing: getListingRpc } });
    await expect(getListing("l1")).rejects.toBeInstanceOf(ConnectError);
  });

  it("suggest short-circuits an empty query without calling the RPC", async () => {
    const suggestRpc = vi.fn();
    stub({ search: { suggest: suggestRpc } });
    await expect(suggest("   ")).resolves.toEqual([]);
    expect(suggestRpc).not.toHaveBeenCalled();
  });
});

describe("storefront display name", () => {
  const protoStorefront = {
    sellerId: "s1",
    slug: "shop-a",
    bannerUrl: "",
    tagline: "",
    featuredListingIds: [],
    theme: "default",
    displayName: "Tiem Hoa Nho",
  };

  it("getStorefront maps displayName", async () => {
    stub({
      listing: {
        getStorefront: vi
          .fn()
          .mockResolvedValue({ storefront: protoStorefront }),
      },
    });
    const sf = await getStorefront("s1");
    expect(sf?.displayName).toBe("Tiem Hoa Nho");
  });
});

describe("listings moderation mapping", () => {
  it.each([
    [ListingStatus.PENDING_REVIEW, "pending_review"],
    [ListingStatus.BANNED, "banned"],
    [ListingStatus.HELD, "held"],
  ])(
    "Moderation UI: Screen flags never reach a seller surface - status %s maps to %s",
    async (status, label) => {
      stub({
        listing: {
          getListing: vi
            .fn()
            .mockResolvedValue({ listing: { ...protoListing, status } }),
        },
      });
      const l = await getListing("l1");
      expect(l?.status).toBe(label);
    },
  );

  it("Moderation UI: Screen flags never reach a seller surface - seller mapping drops screen flags", async () => {
    const moderation = {
      reasonCode: "prohibited_item",
      note: "Vi pham chinh sach",
      screenFlags: ["keyword:thuoc la"],
      decidedAt: { seconds: 1700000000n, nanos: 0 },
    };
    const withFlags = {
      ...protoListing,
      status: ListingStatus.BANNED,
      moderation,
    };
    stub({
      listing: {
        getListing: vi.fn().mockResolvedValue({ listing: withFlags }),
        listListings: vi.fn().mockResolvedValue({
          listings: [withFlags],
          page: { nextCursor: "", total: 1n },
        }),
      },
    });
    const one = await getListing("l1");
    const browse = await listListings();
    for (const l of [one, browse.items[0]]) {
      expect(l?.moderation).toMatchObject({
        reasonCode: "prohibited_item",
        note: "Vi pham chinh sach",
      });
      expect(typeof l?.moderation?.decidedAt).toBe("string");
      expect(l?.moderation?.decidedAt).not.toBe("");
      expect(l?.moderation).not.toHaveProperty("screenFlags");
      expect(JSON.stringify(l)).not.toContain("keyword:");
      expect(l).not.toHaveProperty("screenFlags");
    }
  });

  it("Moderation UI: Screen flags never reach a seller surface - listing without moderation has none", async () => {
    stub({
      listing: {
        getListing: vi.fn().mockResolvedValue({ listing: protoListing }),
      },
    });
    const l = await getListing("l1");
    expect(l?.moderation).toBeUndefined();
  });
});
