import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ListingStatus } from "@/generated/platform/listing/v1/listing_pb.js";

import { makeClients } from "./client.js";
import {
  type CreateListingInput,
  createListing,
  getListing,
  listListings,
  reserveStock,
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

  it("createListing maps prices to bigint and throws when nothing returns", async () => {
    const input: CreateListingInput = {
      title: "Phone",
      description: "desc",
      price: 5000000.9,
      currency: "VND",
      status: "published",
      stock: 2,
      variants: [{ name: "Red", price: 100.7 }],
    };
    const createListingRpc = vi
      .fn()
      .mockResolvedValue({ listing: protoListing });
    stub({ listing: { createListing: createListingRpc } });

    await createListing(input);
    const arg = createListingRpc.mock.calls[0][0];
    expect(arg.listing.price).toBe(5000000n);
    expect(arg.listing.status).toBe(ListingStatus.PUBLISHED);
    expect(arg.listing.variants[0].price).toBe(100n);

    createListingRpc.mockResolvedValueOnce({});
    await expect(createListing(input)).rejects.toThrow(
      "create returned no listing",
    );
  });

  it("suggest short-circuits an empty query without calling the RPC", async () => {
    const suggestRpc = vi.fn();
    stub({ search: { suggest: suggestRpc } });
    await expect(suggest("   ")).resolves.toEqual([]);
    expect(suggestRpc).not.toHaveBeenCalled();
  });

  it("reserveStock normalizes a thrown error to { success: false }", async () => {
    const reserveStockRpc = vi.fn().mockRejectedValue(new Error("out"));
    stub({ listing: { reserveStock: reserveStockRpc } });
    const res = await reserveStock("l1", 1);
    expect(res.success).toBe(false);
    expect(res.message).toContain("out");
  });
});
