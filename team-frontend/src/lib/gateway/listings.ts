/**
 * Server-side data functions the UI calls. They wrap the gateway RPCs and map
 * proto messages to plain view models (bigint → number, enum → label) so React
 * Server Components and route handlers stay free of proto types.
 */
import "server-only";

import { Code, ConnectError } from "@connectrpc/connect";

import {
  type Bundle,
  type Listing,
  ListingStatus,
} from "@/generated/platform/listing/v1/listing_pb.js";

import { getImageUrl } from "../media.js";
import { makeClients } from "./client.js";
import { getToken } from "./session.js";

export { getImageUrl };

// gateway builds request-scoped clients carrying the caller's session token.
function gateway() {
  return makeClients(getToken());
}

export interface ViewCategory {
  id: string;
  name: string;
  slug: string;
  parentId: string;
  displayOrder: number;
  iconUrl: string;
}

export interface ViewVariant {
  id: string;
  listingId: string;
  name: string;
  sku: string;
  price: number;
  stock: number;
  imageUrl: string;
}

export interface ViewListing {
  id: string;
  title: string;
  description: string;
  price: number;
  currency: string;
  status: string;
  sellerId: string;
  imageKeys: string[];
  imageUrl?: string;
  categoryId: string;
  stock: number;
  variants: ViewVariant[];
}

export interface CreateListingInput {
  title: string;
  description: string;
  price: number;
  currency: string;
  status: string; // "draft" | "published" | "rejected"
  imageKeys?: string[];
  categoryId?: string;
  stock?: number;
  variants?: {
    id?: string;
    name: string;
    sku?: string;
    price?: number;
    stock?: number;
    imageUrl?: string;
  }[];
}

export interface ListingPage {
  items: ViewListing[];
  nextCursor: string;
  total: number;
}

function statusLabel(s: ListingStatus): string {
  switch (s) {
    case ListingStatus.DRAFT:
      return "draft";
    case ListingStatus.PUBLISHED:
      return "published";
    case ListingStatus.REJECTED:
      return "rejected";
    default:
      return "unspecified";
  }
}

function statusFromLabel(label: string): ListingStatus {
  switch (label) {
    case "published":
      return ListingStatus.PUBLISHED;
    case "rejected":
      return ListingStatus.REJECTED;
    default:
      return ListingStatus.DRAFT;
  }
}

function mapListing(l: Listing): ViewListing {
  const imageKeys = l.imageKeys ?? [];
  const variants: ViewVariant[] = (l.variants ?? []).map((v) => ({
    id: v.id,
    listingId: v.listingId,
    name: v.name,
    sku: v.sku,
    price: Number(v.price),
    stock: v.stock,
    imageUrl: v.imageUrl,
  }));
  return {
    id: l.id,
    title: l.title,
    description: l.description,
    price: Number(l.price),
    currency: l.currency,
    status: statusLabel(l.status),
    sellerId: l.sellerId,
    imageKeys,
    imageUrl: imageKeys.length > 0 ? getImageUrl(imageKeys[0]) : undefined,
    categoryId: l.categoryId,
    stock: l.stock,
    variants,
  };
}

/** Consumer browse: full listings, filtered by status, keyset-paginated. */
export async function listListings(
  opts: { status?: string; cursor?: string; pageSize?: number } = {},
): Promise<ListingPage> {
  const res = await gateway().listing.listListings({
    status: opts.status ?? "",
    page: { cursor: opts.cursor ?? "", pageSize: opts.pageSize ?? 12 },
  });
  return {
    items: res.listings.map(mapListing),
    nextCursor: res.page?.nextCursor ?? "",
    total: Number(res.page?.total ?? 0n),
  };
}

/** Seller center: the caller's OWN listings. */
export async function listMyListings(
  opts: { cursor?: string; pageSize?: number } = {},
): Promise<ListingPage> {
  const res = await gateway().listing.listMyListings({
    page: { cursor: opts.cursor ?? "", pageSize: opts.pageSize ?? 20 },
  });
  return {
    items: res.listings.map(mapListing),
    nextCursor: res.page?.nextCursor ?? "",
    total: Number(res.page?.total ?? 0n),
  };
}

/** Fetch one listing, or null if it no longer exists. */
export async function getListing(id: string): Promise<ViewListing | null> {
  try {
    const res = await gateway().listing.getListing({ id });
    return res.listing ? mapListing(res.listing) : null;
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.NotFound) return null;
    throw err;
  }
}

import { SortBy } from "@/generated/platform/search/v1/search_pb.js";

export interface SearchResult {
  items: ViewListing[];
  total: number;
}

/**
 * Free-text + filter search. SearchService returns listing ids; we resolve each
 * to a full listing via the listing service so the UI can render cards.
 */
export async function searchListings(
  query: string,
  opts: {
    status?: string;
    categoryId?: string;
    minPrice?: number;
    maxPrice?: number;
    sortBy?: SortBy;
  } = {},
): Promise<SearchResult> {
  const filters: Record<string, string> = {};
  filters.status = opts.status ?? "published";
  if (opts.categoryId) filters.category_id = opts.categoryId;

  const res = await gateway().search.searchListings({
    query,
    filters,
    categoryId: opts.categoryId ?? "",
    minPrice: opts.minPrice ? BigInt(opts.minPrice) : 0n,
    maxPrice: opts.maxPrice ? BigInt(opts.maxPrice) : 0n,
    sortBy: opts.sortBy ?? SortBy.UNSPECIFIED,
  });
  const resolved = await Promise.all(
    res.hits.map((h) => getListing(h.listingId)),
  );
  return {
    items: resolved.filter(
      (l): l is ViewListing =>
        l !== null && (l.status === "published" || !l.status),
    ),
    total: Number(res.page?.total ?? 0n),
  };
}

/** Type-ahead suggestions (listing titles). */
export async function suggest(query: string, limit = 8): Promise<string[]> {
  if (!query.trim()) return [];
  const res = await gateway().search.suggest({ query, limit });
  return res.suggestions;
}

/** Fetch categories tree or list. */
export async function listCategories(parentId = ""): Promise<ViewCategory[]> {
  try {
    const res = await gateway().listing.listCategories({ parentId });
    return res.categories.map((c) => ({
      id: c.id,
      name: c.name,
      slug: c.slug,
      parentId: c.parentId,
      displayOrder: c.displayOrder,
      iconUrl: c.iconUrl,
    }));
  } catch {
    return [];
  }
}

/** Fetch single category by id. */
export async function getCategory(id: string): Promise<ViewCategory | null> {
  try {
    const res = await gateway().listing.getCategory({ id });
    if (!res.category) return null;
    return {
      id: res.category.id,
      name: res.category.name,
      slug: res.category.slug,
      parentId: res.category.parentId,
      displayOrder: res.category.displayOrder,
      iconUrl: res.category.iconUrl,
    };
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.NotFound) return null;
    return null;
  }
}

/** Seller posts a listing; the service persists it and emits an event. */
export async function createListing(
  input: CreateListingInput,
): Promise<ViewListing> {
  const res = await gateway().listing.createListing({
    listing: {
      title: input.title,
      description: input.description,
      price: BigInt(Math.trunc(input.price)),
      currency: input.currency,
      status: statusFromLabel(input.status),
      imageKeys: input.imageKeys ?? [],
      categoryId: input.categoryId ?? "",
      stock: input.stock ?? 0,
      variants: (input.variants ?? []).map((v) => ({
        id: v.id ?? "",
        listingId: "",
        name: v.name,
        sku: v.sku ?? "",
        price: BigInt(Math.trunc(v.price ?? 0)),
        stock: v.stock ?? 0,
        imageUrl: v.imageUrl ?? "",
      })),
    },
  });
  if (!res.listing) throw new Error("create returned no listing");
  return mapListing(res.listing);
}

/** Seller updates one of their own listings (server enforces ownership). */
export async function updateListing(
  id: string,
  input: CreateListingInput,
): Promise<ViewListing> {
  const res = await gateway().listing.updateListing({
    listing: {
      id,
      title: input.title,
      description: input.description,
      price: BigInt(Math.trunc(input.price)),
      currency: input.currency,
      status: statusFromLabel(input.status),
      imageKeys: input.imageKeys ?? [],
      categoryId: input.categoryId ?? "",
      stock: input.stock ?? 0,
      variants: (input.variants ?? []).map((v) => ({
        id: v.id ?? "",
        listingId: id,
        name: v.name,
        sku: v.sku ?? "",
        price: BigInt(Math.trunc(v.price ?? 0)),
        stock: v.stock ?? 0,
        imageUrl: v.imageUrl ?? "",
      })),
    },
  });
  if (!res.listing) throw new Error("update returned no listing");
  return mapListing(res.listing);
}

/** Seller removes one of their own listings (server enforces ownership). */
export async function deleteListing(id: string): Promise<void> {
  await gateway().listing.deleteListing({ id });
}

/** Request a presigned URL to upload a product image. */
export async function getImageUploadUrl(
  contentType: string,
  filename?: string,
): Promise<{ uploadUrl: string; imageKey: string; publicUrl: string }> {
  const res = await gateway().listing.getImageUploadUrl({
    contentType,
    filename: filename ?? "",
  });
  return {
    uploadUrl: res.uploadUrl,
    imageKey: res.imageKey,
    publicUrl: res.publicUrl,
  };
}

/** Atomically reserve inventory for checkout. */
export async function reserveStock(
  listingId: string,
  quantity: number,
  variantId = "",
  reservationId = "",
): Promise<{ success: boolean; message?: string }> {
  try {
    const res = await gateway().listing.reserveStock({
      listingId,
      variantId,
      quantity,
      reservationId,
    });
    return { success: res.success, message: res.message };
  } catch (err) {
    return { success: false, message: String(err) };
  }
}

/** Release previously reserved inventory. */
export async function releaseStock(
  listingId: string,
  quantity: number,
  variantId = "",
  reservationId = "",
): Promise<boolean> {
  try {
    const res = await gateway().listing.releaseStock({
      listingId,
      variantId,
      quantity,
      reservationId,
    });
    return res.success;
  } catch {
    return false;
  }
}

// ── Storefront (seller shop banner / tagline / featured items) ──────────────

export interface ViewStorefront {
  sellerId: string;
  slug: string;
  bannerUrl: string;
  tagline: string;
  featuredListingIds: string[];
  theme: string;
}

export async function getStorefront(
  sellerId: string,
): Promise<ViewStorefront | null> {
  try {
    const res = await gateway().listing.getStorefront({ sellerId });
    const s = res.storefront;
    if (!s) return null;
    return {
      sellerId: s.sellerId || sellerId,
      slug: s.slug,
      bannerUrl: s.bannerUrl,
      tagline: s.tagline,
      featuredListingIds: s.featuredListingIds ?? [],
      theme: s.theme,
    };
  } catch {
    return null;
  }
}

export async function upsertStorefront(
  input: Partial<ViewStorefront> & { sellerId: string },
): Promise<ViewStorefront> {
  const res = await gateway().listing.upsertStorefront({
    storefront: {
      sellerId: input.sellerId,
      slug: input.slug ?? "",
      bannerUrl: input.bannerUrl ?? "",
      tagline: input.tagline ?? "",
      featuredListingIds: input.featuredListingIds ?? [],
      theme: input.theme ?? "",
    },
  });
  const s = res.storefront;
  if (!s) throw new Error("upsert storefront failed");
  return {
    sellerId: s.sellerId,
    slug: s.slug,
    bannerUrl: s.bannerUrl,
    tagline: s.tagline,
    featuredListingIds: s.featuredListingIds ?? [],
    theme: s.theme,
  };
}

// ── Listing bundles ─────────────────────────────────────────────────────────
// A bundle groups several of a seller's listings for one bundle price.
// team-listing owns validation/pricing; this module forwards + maps.

export interface ViewBundle {
  id: string;
  sellerId: string;
  title: string;
  listingIds: string[];
  bundlePrice: number;
  createdAt: string;
}

function mapBundle(b: Bundle): ViewBundle {
  let createdAt = "";
  if (b.createdAt) {
    createdAt = new Date(Number(b.createdAt.seconds) * 1000).toLocaleDateString(
      "vi-VN",
    );
  }
  return {
    id: b.id,
    sellerId: b.sellerId,
    title: b.title,
    listingIds: b.listingIds ?? [],
    bundlePrice: Number(b.bundlePrice),
    createdAt,
  };
}

export async function createBundle(
  title: string,
  listingIds: string[],
  bundlePrice: number,
): Promise<ViewBundle> {
  const res = await gateway().listing.createBundle({
    title: title.trim(),
    listingIds,
    bundlePrice: BigInt(Math.max(0, Math.round(bundlePrice))),
  });
  if (!res.bundle) throw new Error("create bundle failed");
  return mapBundle(res.bundle);
}

export async function getBundle(id: string): Promise<ViewBundle | null> {
  try {
    const res = await gateway().listing.getBundle({ id });
    return res.bundle ? mapBundle(res.bundle) : null;
  } catch {
    return null;
  }
}

export async function listBundlesBySeller(
  sellerId: string,
): Promise<ViewBundle[]> {
  try {
    const res = await gateway().listing.listBundlesBySeller({ sellerId });
    return res.bundles.map(mapBundle);
  } catch {
    return [];
  }
}
