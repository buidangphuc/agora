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

function clientsWith(impl: {
  listListings?: () => unknown;
  getListing?: () => unknown;
}) {
  return { listing: impl } as unknown as Clients;
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
