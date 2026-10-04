/**
 * Pure product-detail-page logic: variant resolution, URL state and review
 * pagination. No server-only or React imports so the server page and the
 * client islands (VariantSelector, PurchaseProvider) share one definition.
 */
import type { ViewListing, ViewVariant } from "@/lib/gateway/listings";

export const REVIEWS_PAGE_SIZE = 10;
const MAX_PAGE = 10_000;

export type StorefrontSort = "all" | "price_asc" | "price_desc";

export interface PdpParams {
  /** Selected variant id from `?variant=`, undefined when absent. */
  variant: string | undefined;
  /** Star filter from `?rating=`: 0 = all, otherwise 1..5. */
  rating: number;
  /** 1-based reviews page from `?rpage=`. */
  rpage: number;
  /** Price sort from `?sort=` (shop storefront). */
  sort: StorefrontSort;
}

type RawParams = Record<string, string | string[] | undefined>;

function first(value: string | string[] | undefined): string | undefined {
  const v = Array.isArray(value) ? value[0] : value;
  return v === undefined || v === "" ? undefined : v;
}

function parseInteger(value: string | undefined): number | null {
  if (value === undefined || !/^\d+$/.test(value.trim())) return null;
  return Number(value);
}

/** `?sort=` -> a known sort, "all" for anything else. */
export function parseSort(
  value: string | string[] | undefined,
): StorefrontSort {
  const v = first(value);
  return v === "price_asc" || v === "price_desc" ? v : "all";
}

/** Read the PDP URL state with defaults and bounds. There is no tab param. */
export function parsePdpParams(searchParams: RawParams = {}): PdpParams {
  const rating = parseInteger(first(searchParams.rating));
  const rpage = parseInteger(first(searchParams.rpage));
  return {
    variant: first(searchParams.variant)?.trim() || undefined,
    rating: rating !== null && rating >= 1 && rating <= 5 ? rating : 0,
    rpage: rpage !== null && rpage >= 1 ? Math.min(rpage, MAX_PAGE) : 1,
    sort: parseSort(searchParams.sort),
  };
}

export interface ResolvedVariant {
  /** The chosen variant, null when the listing has none (base listing). */
  variant: ViewVariant | null;
  /** Variant id, "" for the base listing. */
  id: string;
  /** Unit price: the variant's own price when set, else the base price. */
  price: number;
  stock: number;
  sku: string;
  /** Variant image URL, "" when it has none. */
  imageUrl: string;
}

function toResolved(
  listing: Pick<ViewListing, "price" | "stock">,
  variant: ViewVariant | null,
): ResolvedVariant {
  if (!variant) {
    return {
      variant: null,
      id: "",
      price: listing.price,
      stock: listing.stock,
      sku: "",
      imageUrl: "",
    };
  }
  return {
    variant,
    id: variant.id,
    price: variant.price > 0 ? variant.price : listing.price,
    stock: variant.stock,
    sku: variant.sku,
    imageUrl: variant.imageUrl,
  };
}

/**
 * The variant the page shows for `?variant=<id>`. An unknown or missing id falls
 * back to the first in-stock variant, else the first variant, else the base
 * listing. A known id wins even when it is out of stock.
 */
export function resolveVariant(
  listing: Pick<ViewListing, "price" | "stock" | "variants">,
  variantId?: string | null,
): ResolvedVariant {
  const variants = listing.variants ?? [];
  if (variants.length === 0) return toResolved(listing, null);
  const requested = variantId
    ? variants.find((v) => v.id === variantId)
    : undefined;
  const chosen =
    requested ?? variants.find((v) => v.stock > 0) ?? variants[0] ?? null;
  return toResolved(listing, chosen);
}

/**
 * Whether selecting this variant is worth a URL entry: only when its price or
 * stock differs from the base listing. Otherwise the selection stays local.
 */
export function shouldWriteVariantToUrl(
  listing: Pick<ViewListing, "price" | "stock" | "variants">,
  variantId: string,
): boolean {
  const variant = (listing.variants ?? []).find((v) => v.id === variantId);
  if (!variant) return false;
  const resolved = toResolved(listing, variant);
  return resolved.price !== listing.price || resolved.stock !== listing.stock;
}

export interface Page<T> {
  items: T[];
  /** Current page, clamped to 1..pages. */
  page: number;
  pages: number;
  total: number;
}

/** Slice `items` into 1-based pages of `size`. */
export function paginate<T>(items: T[], page: number, size: number): Page<T> {
  const safeSize = Math.max(1, Math.floor(size));
  const pages = Math.max(1, Math.ceil(items.length / safeSize));
  const current = Math.min(Math.max(1, Math.floor(page) || 1), pages);
  const start = (current - 1) * safeSize;
  return {
    items: items.slice(start, start + safeSize),
    page: current,
    pages,
    total: items.length,
  };
}

/**
 * Build a PDP URL from the current state plus overrides. Defaults (rating 0,
 * rpage 1, no variant) are omitted; `hash` is appended as `#hash`.
 */
export function pdpHref(
  listingId: string,
  state: Partial<Pick<PdpParams, "variant" | "rating" | "rpage">>,
  hash?: string,
): string {
  const qs = new URLSearchParams();
  if (state.variant) qs.set("variant", state.variant);
  if (state.rating && state.rating >= 1 && state.rating <= 5) {
    qs.set("rating", String(state.rating));
  }
  if (state.rpage && state.rpage > 1) qs.set("rpage", String(state.rpage));
  const query = qs.toString();
  return `/listing/${listingId}${query ? `?${query}` : ""}${hash ? `#${hash}` : ""}`;
}

/** Count of reviews with the given star value from a rating breakdown. */
export function starCount(
  breakdown: {
    star1: number;
    star2: number;
    star3: number;
    star4: number;
    star5: number;
  },
  star: number,
): number {
  switch (star) {
    case 5:
      return breakdown.star5;
    case 4:
      return breakdown.star4;
    case 3:
      return breakdown.star3;
    case 2:
      return breakdown.star2;
    default:
      return breakdown.star1;
  }
}
