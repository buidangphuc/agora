import { ApiError, ListingStatus } from "@fe-platform/api-client";
import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  notFound: () => {
    throw new Error("NEXT_NOT_FOUND");
  },
  redirect: (to: string) => {
    throw new Error(`NEXT_REDIRECT:${to}`);
  },
}));
vi.mock("next/headers", () => ({ cookies: vi.fn() }));

import {
  type Clients,
  formatPrice,
  loadProduct,
  loadProductPage,
  loadProducts,
  toProductView,
} from "./products";

const listing = (over: Record<string, unknown> = {}) =>
  ({
    id: "p1",
    title: "Lamp",
    description: "Desk lamp",
    price: 250000n,
    currency: "VND",
    status: ListingStatus.PUBLISHED,
    ...over,
  }) as never;

function clientsWith(
  impl: {
    listListings?: () => unknown;
    getListing?: (r: { id: string }) => unknown;
  },
  search?: { searchListings: (r: { query: string }) => unknown },
) {
  return { listing: impl, search } as unknown as Clients;
}

describe("toProductView", () => {
  it("formats a bigint minor-unit price and flags published", () => {
    const v = toProductView(listing());
    expect(v.price).toContain("250,000");
    expect(v.published).toBe(true);
    expect(
      toProductView(listing({ status: ListingStatus.DRAFT })).published,
    ).toBe(false);
  });

  it("falls back for an unknown currency code", () => {
    expect(formatPrice(5n, "??")).toBe("5 ??");
  });
});

describe("loadProducts", () => {
  it("maps an empty list", async () => {
    const clients = clientsWith({
      listListings: async () => ({ listings: [] }),
    });
    await expect(loadProducts(clients)).resolves.toEqual([]);
  });

  it("maps listings", async () => {
    const clients = clientsWith({
      listListings: async () => ({ listings: [listing()] }),
    });
    const [p] = await loadProducts(clients);
    expect(p.id).toBe("p1");
  });

  it("throws an ApiError on failure", async () => {
    const clients = clientsWith({
      listListings: async () => {
        throw new ApiError("unavailable", "down");
      },
    });
    await expect(loadProducts(clients)).rejects.toSatisfy(
      (e) => e instanceof ApiError && e.kind === "unavailable",
    );
  });
});

describe("loadProduct", () => {
  it("calls notFound for a not_found error", async () => {
    const clients = clientsWith({
      getListing: async () => {
        throw new ApiError("not_found", "no");
      },
    });
    await expect(loadProduct(clients, "x")).rejects.toThrow("NEXT_NOT_FOUND");
  });

  it("calls notFound when the response has no listing", async () => {
    const clients = clientsWith({ getListing: async () => ({}) });
    await expect(loadProduct(clients, "x")).rejects.toThrow("NEXT_NOT_FOUND");
  });

  it("rethrows other errors as ApiError", async () => {
    const clients = clientsWith({
      getListing: async () => {
        throw new ApiError("forbidden", "denied");
      },
    });
    await expect(loadProduct(clients, "x")).rejects.toSatisfy(
      (e) => e instanceof ApiError && e.kind === "forbidden",
    );
  });

  it("returns the view for a found listing", async () => {
    const clients = clientsWith({
      getListing: async () => ({ listing: listing() }),
    });
    await expect(loadProduct(clients, "p1")).resolves.toMatchObject({
      id: "p1",
      title: "Lamp",
    });
  });
});

describe("loadProducts search", () => {
  it("lists published listings when the query is blank", async () => {
    const listListings = vi.fn(async () => ({ listings: [listing()] }));
    const searchListings = vi.fn();
    const out = await loadProducts(
      clientsWith({ listListings }, { searchListings }),
      "  ",
    );
    expect(out).toHaveLength(1);
    expect(searchListings).not.toHaveBeenCalled();
  });

  it("searches, fetches each hit and skips a hit that is gone", async () => {
    const searchListings = vi.fn(async () => ({
      hits: [{ listingId: "p1" }, { listingId: "gone" }, { listingId: "p2" }],
    }));
    const getListing = vi.fn(async ({ id }: { id: string }) => {
      if (id === "gone") throw new ApiError("not_found", "gone");
      return { listing: listing({ id, title: id }) };
    });
    const out = await loadProducts(
      clientsWith({ getListing }, { searchListings }),
      " lamp ",
    );
    expect(searchListings).toHaveBeenCalledWith({ query: "lamp" });
    expect(out.map((p) => p.id)).toEqual(["p1", "p2"]);
  });

  it("fetches at most 20 hits", async () => {
    const hits = Array.from({ length: 30 }, (_, i) => ({ listingId: `p${i}` }));
    const getListing = vi.fn(async ({ id }: { id: string }) => ({
      listing: listing({ id }),
    }));
    await loadProducts(
      clientsWith({ getListing }, { searchListings: async () => ({ hits }) }),
      "x",
    );
    expect(getListing).toHaveBeenCalledTimes(20);
  });

  it("maps a search failure to an ApiError", async () => {
    await expect(
      loadProducts(
        clientsWith(
          {},
          {
            searchListings: async () => {
              throw new ApiError("unavailable", "down");
            },
          },
        ),
        "x",
      ),
    ).rejects.toMatchObject({ kind: "unavailable" });
  });
});

describe("toProductView variants", () => {
  it("maps variants and falls back to the listing price", () => {
    const v = toProductView(
      listing({
        variants: [
          { id: "v1", name: "Blue", price: 0n, stock: 3 },
          { id: "v2", name: "Red", price: 300000n, stock: 0 },
        ],
        stock: 3,
      }),
    );
    expect(v.variants.map((x) => x.name)).toEqual(["Blue", "Red"]);
    expect(v.variants[0]?.price).toContain("250,000");
    expect(v.variants[1]?.price).toContain("300,000");
    expect(v.stock).toBe(3);
  });
});

describe("loadProductPage", () => {
  it("passes the cursor and returns the next one", async () => {
    const listListings = vi.fn(async () => ({
      listings: [listing()],
      page: { nextCursor: "c2" },
    }));
    const out = await loadProductPage(clientsWith({ listListings }), {
      cursor: "c1",
    });
    expect(listListings).toHaveBeenCalledWith({
      status: "published",
      page: { cursor: "c1", pageSize: 0 },
    });
    expect(out.nextCursor).toBe("c2");
  });

  it("returns an empty cursor on the last page", async () => {
    const out = await loadProductPage(
      clientsWith({ listListings: async () => ({ listings: [] }) }),
    );
    expect(out).toEqual({ items: [], nextCursor: "" });
  });
});
