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
  AttributeFacet,
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

// Public reads: retry once without the bearer if the gateway rejects a stale
// session (it answers Unauthenticated to any invalid token, even on public RPCs).
function publicGateway() {
  return makeClients(getToken(), { anonymousFallback: true });
}

export interface ViewFacetBucket {
  key: string;
  count: number;
}

/** One dynamic facet group (e.g. "color") with its tag-slug buckets. */
export interface ViewAttributeFacet {
  group: string;
  buckets: ViewFacetBucket[];
}

export interface ViewFacets {
  categories: ViewFacetBucket[];
  priceRanges: ViewFacetBucket[];
  ratings: ViewFacetBucket[];
  sellers: ViewFacetBucket[];
  /** SPU-level canonical tag facets; filter key `tag.<group>`. */
  tags: ViewAttributeFacet[];
  /** Variant (nested SKU) facets; filter key `sku.<group>`. */
  skus: ViewAttributeFacet[];
}

export interface FacetedSearchResult {
  items: ViewListing[];
  total: number;
  facets: ViewFacets;
  /** The page actually returned (1-based); less than the requested page when it was out of range. */
  page: number;
  pageSize: number;
}

/** Results per page and the deepest page `searchListings` will walk to. */
export const SEARCH_PAGE_SIZE = 24;
export const SEARCH_MAX_PAGE = 20;

export const EMPTY_FACETS: ViewFacets = {
  categories: [],
  priceRanges: [],
  ratings: [],
  sellers: [],
  tags: [],
  skus: [],
};

function mapBuckets(
  buckets: { key: string; count: bigint }[],
): ViewFacetBucket[] {
  return buckets.map((b) => ({ key: b.key, count: Number(b.count) }));
}

function mapAttributes(groups: AttributeFacet[]): ViewAttributeFacet[] {
  return groups
    .filter((g) => g.group && g.buckets.length > 0)
    .map((g) => ({ group: g.group, buckets: mapBuckets(g.buckets) }));
}

function mapFacets(f?: Facets): ViewFacets {
  if (!f) return EMPTY_FACETS;
  return {
    categories: mapBuckets(f.categories),
    priceRanges: mapBuckets(f.priceRanges),
    ratings: mapBuckets(f.ratings),
    sellers: mapBuckets(f.sellers),
    tags: mapAttributes(f.tags),
    skus: mapAttributes(f.skus),
  };
}

export interface SearchOptions {
  status?: string;
  categoryId?: string;
  sellerId?: string;
  minPrice?: number;
  maxPrice?: number;
  sortBy?: SortBy;
  /** Dynamic facet selections: filter key (`tag.x` / `sku.x`) -> tag slugs. */
  attrs?: Record<string, string[]>;
  /** 1-based page; resolved by walking next_cursor (capped at SEARCH_MAX_PAGE). */
  page?: number;
}

/**
 * Free-text + faceted search. SearchService returns listing ids plus facet
 * aggregations over the matched set; we resolve each hit to a full listing via
 * the listing service so the UI can render cards, and pass the facet counts
 * through for filter navigation. Facet selections arrive as request filters
 * (category_id / min_price / max_price / seller_id).
 */
export async function searchListings(
  query: string,
  opts: SearchOptions = {},
): Promise<FacetedSearchResult> {
  const filters: Record<string, string> = {};
  filters.status = opts.status ?? "published";
  if (opts.categoryId) filters.category_id = opts.categoryId;
  if (opts.sellerId) filters.seller_id = opts.sellerId;
  for (const [key, slugs] of Object.entries(opts.attrs ?? {})) {
    if (slugs.length > 0) filters[key] = slugs.join(",");
  }

  const wanted = Math.min(
    SEARCH_MAX_PAGE,
    Math.max(1, Math.floor(opts.page ?? 1) || 1),
  );
  const search = publicGateway().search;
  const request = {
    query,
    filters,
    categoryId: opts.categoryId ?? "",
    minPrice: opts.minPrice
      ? BigInt(Math.max(0, Math.round(opts.minPrice)))
      : 0n,
    maxPrice: opts.maxPrice
      ? BigInt(Math.max(0, Math.round(opts.maxPrice)))
      : 0n,
    // Search indexes no rating: a minimum rating is never requested.
    minRating: 0,
    sortBy: opts.sortBy ?? SortBy.UNSPECIFIED,
  };

  // The RPC pages by opaque cursor: walk next_cursor to the wanted page. When
  // the cursor runs out first, the last page that exists is returned (the
  // caller compares `page` with what it asked for and redirects).
  let cursor = "";
  let page = 1;
  let res = await search.searchListings({
    ...request,
    page: { cursor, pageSize: SEARCH_PAGE_SIZE },
  });
  while (page < wanted && res.page?.nextCursor) {
    cursor = res.page.nextCursor;
    page += 1;
    res = await search.searchListings({
      ...request,
      page: { cursor, pageSize: SEARCH_PAGE_SIZE },
    });
  }

  const resolved = await Promise.all(
    res.hits.map((h) => getListing(h.listingId)),
  );
  const items = resolved.filter(
    (l): l is ViewListing =>
      l !== null && (l.status === "published" || !l.status),
  );

  // `total` is best-effort (-1 when unknown): fall back to a lower bound that
  // still lets Pagination show a "next" link.
  const reported = Number(res.page?.total ?? -1n);
  const total =
    reported >= 0
      ? reported
      : (page - 1) * SEARCH_PAGE_SIZE +
        res.hits.length +
        (res.page?.nextCursor ? 1 : 0);

  return {
    items,
    total,
    facets: mapFacets(res.facets),
    page,
    pageSize: SEARCH_PAGE_SIZE,
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
