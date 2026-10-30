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

export type ProductView = {
  id: string;
  title: string;
  description: string;
  price: string;
  published: boolean;
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
  };
}

export async function gatewayClients(): Promise<Clients> {
  return createGatewayClients({
    token: await getToken(),
    anonymousFallback: true,
  });
}

export async function loadProducts(clients: Clients): Promise<ProductView[]> {
  try {
    const res = await clients.listing.listListings({ status: "published" });
    return res.listings.map(toProductView);
  } catch (err) {
    throw toApiError(err);
  }
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
