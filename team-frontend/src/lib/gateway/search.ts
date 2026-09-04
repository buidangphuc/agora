/**
 * Server-only gateway module for team-search (SearchService), reached — like
 * every other domain — ONLY through the gateway (ARCHITECTURE Rule 1). Clients
 * are built per request with the caller's session token, mirroring orders.ts /
 * promotion.ts. No business logic lives here: relevance + facet aggregation are
 * computed by team-search; this module forwards the request and maps proto →
 * plain view types the RSC search page can render.
 */
import "server-only";

import { SortBy } from "@/generated/platform/search/v1/search_pb.js";
import type {
  Facets,
  SavedSearch,
} from "@/generated/platform/search/v1/search_pb.js";

import { makeClients } from "./client.js";
import { type ViewListing, getListing } from "./listings.js";
import { getToken } from "./session.js";

export { SortBy };

// Request-scoped gateway clients carrying the caller's bearer (anonymous when
// absent — the gateway grants public search scopes).
function gateway() {
  return makeClients(getToken());
}

export interface ViewFacetBucket {
  key: string;
  count: number;
}

export interface ViewFacets {
  categories: ViewFacetBucket[];
  priceRanges: ViewFacetBucket[];
  ratings: ViewFacetBucket[];
  sellers: ViewFacetBucket[];
}

export interface FacetedSearchResult {
  items: ViewListing[];
  total: number;
  facets: ViewFacets;
}

export const EMPTY_FACETS: ViewFacets = {
  categories: [],
  priceRanges: [],
  ratings: [],
  sellers: [],
};

function mapBuckets(
  buckets: { key: string; count: bigint }[],
): ViewFacetBucket[] {
  return buckets.map((b) => ({ key: b.key, count: Number(b.count) }));
}

function mapFacets(f?: Facets): ViewFacets {
  if (!f) return EMPTY_FACETS;
  return {
    categories: mapBuckets(f.categories),
    priceRanges: mapBuckets(f.priceRanges),
    ratings: mapBuckets(f.ratings),
    sellers: mapBuckets(f.sellers),
  };
}

export interface SearchOptions {
  status?: string;
  categoryId?: string;
  sellerId?: string;
  minPrice?: number;
  maxPrice?: number;
  minRating?: number;
  sortBy?: SortBy;
}

/**
 * Free-text + faceted search. SearchService returns listing ids plus facet
 * aggregations over the matched set; we resolve each hit to a full listing via
 * the listing service so the UI can render cards, and pass the facet counts
 * through for filter navigation. Facet selections arrive as request filters
 * (category_id / min_price / max_price / min_rating / seller_id).
 */
export async function searchListings(
  query: string,
  opts: SearchOptions = {},
): Promise<FacetedSearchResult> {
  const filters: Record<string, string> = {};
  filters.status = opts.status ?? "published";
  if (opts.categoryId) filters.category_id = opts.categoryId;
  if (opts.sellerId) filters.seller_id = opts.sellerId;

  const res = await gateway().search.searchListings({
    query,
    filters,
    categoryId: opts.categoryId ?? "",
    minPrice: opts.minPrice
      ? BigInt(Math.max(0, Math.round(opts.minPrice)))
      : 0n,
    maxPrice: opts.maxPrice
      ? BigInt(Math.max(0, Math.round(opts.maxPrice)))
      : 0n,
    minRating: opts.minRating ? Math.max(0, Math.round(opts.minRating)) : 0,
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
    facets: mapFacets(res.facets),
  };
}

// ── Saved searches ──────────────────────────────────────────────────────────
// team-search persists a buyer's queries; the UI lets them re-run a saved query
// later. filtersJson carries the serialized filter selections opaque to the UI.

export interface ViewSavedSearch {
  id: string;
  query: string;
  filtersJson: string;
  createdAt: string;
}

function mapSavedSearch(s: SavedSearch): ViewSavedSearch {
  let createdAt = "";
  if (s.createdAt) {
    createdAt = new Date(Number(s.createdAt.seconds) * 1000).toLocaleDateString(
      "vi-VN",
    );
  }
  return {
    id: s.id,
    query: s.query,
    filtersJson: s.filtersJson,
    createdAt,
  };
}

export async function saveSearch(
  query: string,
  filtersJson = "",
): Promise<ViewSavedSearch> {
  const res = await gateway().search.saveSearch({ query, filtersJson });
  if (!res.savedSearch) throw new Error("save search failed");
  return mapSavedSearch(res.savedSearch);
}

export async function listSavedSearches(): Promise<ViewSavedSearch[]> {
  try {
    const res = await gateway().search.listSavedSearches({});
    return res.savedSearches.map(mapSavedSearch);
  } catch {
    return [];
  }
}

export async function deleteSavedSearch(id: string): Promise<void> {
  await gateway().search.deleteSavedSearch({ id });
}

/** Re-run a saved search; returns resolved listing cards. */
export async function runSavedSearch(id: string): Promise<ViewListing[]> {
  try {
    const res = await gateway().search.runSavedSearch({ id });
    const resolved = await Promise.all(
      res.hits.map((h) => getListing(h.listingId)),
    );
    return resolved.filter((l): l is ViewListing => l !== null);
  } catch {
    return [];
  }
}
