import "server-only";

import {
  type Listing,
  ListingStatus,
  createGatewayClients,
  toApiError,
} from "@fe-platform/api-client";
import { notFound } from "next/navigation";

import { getToken } from "./session";

export type Clients = ReturnType<typeof createGatewayClients>;

export type VariantView = {
  id: string;
  name: string;
  price: string;
  stock: number;
};

export type ProductView = {
  id: string;
  title: string;
  description: string;
  price: string;
  published: boolean;
  stock: number;
  variants: VariantView[];
};

export function formatPrice(minor: bigint | string | number, currency: string) {
  const amount = Number(minor);
  try {
    return new Intl.NumberFormat("en", {
      style: "currency",
      currency: currency || "VND",
      maximumFractionDigits: 0,
    }).format(amount);
  } catch {
    return `${amount} ${currency}`;
  }
}

export function toProductView(l: Listing): ProductView {
  return {
    id: l.id,
    title: l.title,
    description: l.description,
    price: formatPrice(l.price, l.currency),
    published: l.status === ListingStatus.PUBLISHED,
    stock: l.stock ?? 0,
    variants: (l.variants ?? []).map((v) => ({
      id: v.id,
      name: v.name,
      price: formatPrice(v.price || l.price, l.currency),
      stock: v.stock,
    })),
  };
}

export async function gatewayClients(): Promise<Clients> {
  return createGatewayClients({
    token: await getToken(),
    anonymousFallback: true,
  });
}

const SEARCH_HITS = 20;

export type ProductPage = { items: ProductView[]; nextCursor: string };

export async function loadProductPage(
  clients: Clients,
  opts: { query?: string; cursor?: string } = {},
): Promise<ProductPage> {
  const q = opts.query?.trim();
  const page = opts.cursor ? { cursor: opts.cursor, pageSize: 0 } : undefined;
  try {
    if (!q) {
      const res = await clients.listing.listListings({
        status: "published",
        page,
      });
      return {
        items: res.listings.map(toProductView),
        nextCursor: res.page?.nextCursor ?? "",
      };
    }
    const found = await clients.search.searchListings({ query: q, page });
    const settled = await Promise.all(
      found.hits.slice(0, SEARCH_HITS).map(async (h) => {
        try {
          return (await clients.listing.getListing({ id: h.listingId }))
            .listing;
        } catch (err) {
          if (toApiError(err).kind === "not_found") return undefined;
          throw err;
        }
      }),
    );
    return {
      items: settled
        .filter((l): l is Listing => l !== undefined)
        .map(toProductView),
      nextCursor: found.page?.nextCursor ?? "",
    };
  } catch (err) {
    throw toApiError(err);
  }
}

export async function loadProducts(
  clients: Clients,
  query?: string,
): Promise<ProductView[]> {
  return (await loadProductPage(clients, { query })).items;
}

export async function loadProduct(
  clients: Clients,
  id: string,
): Promise<ProductView> {
  let listing: Listing | undefined;
  try {
    listing = (await clients.listing.getListing({ id })).listing;
  } catch (err) {
    const e = toApiError(err);
    if (e.kind === "not_found") notFound();
    throw e;
  }
  if (!listing) notFound();
  return toProductView(listing);
}
