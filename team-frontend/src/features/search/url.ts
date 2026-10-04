/**
 * URL state of /search (UI_SYSTEM_DESIGN.md section 5.B): `q, category, seller,
 * rating, minPrice, maxPrice, sort, page`. Pure helpers (no React, no gateway)
 * so the server page, the link-based filters and the tests share one parser and
 * one builder. Unknown or malformed values fall back to defaults, never throw.
 */

export type SortKey = "relevance" | "newest" | "price_asc" | "price_desc";

export const SORT_KEYS: readonly SortKey[] = [
  "relevance",
  "newest",
  "price_asc",
  "price_desc",
];

export interface SearchState {
  q: string;
  category: string;
  seller: string;
  /** "1".."5" or "" when not filtering by rating. */
  rating: string;
  minPrice?: number;
  maxPrice?: number;
  sort: SortKey;
  /** 1-based. */
  page: number;
}

export type RawSearchParams = Record<string, string | string[] | undefined>;

function first(v: string | string[] | undefined): string {
  const s = Array.isArray(v) ? v[0] : v;
  return (s ?? "").trim();
}

function positiveInt(v: string): number | undefined {
  if (!/^\d+$/.test(v)) return undefined;
  const n = Number(v);
  return Number.isSafeInteger(n) && n > 0 ? n : undefined;
}

export function parseSearchParams(raw: RawSearchParams): SearchState {
  const rating = first(raw.rating);
  const sort = first(raw.sort);
  return {
    q: first(raw.q),
    category: first(raw.category),
    seller: first(raw.seller),
    rating: /^[1-5]$/.test(rating) ? rating : "",
    minPrice: positiveInt(first(raw.minPrice)),
    maxPrice: positiveInt(first(raw.maxPrice)),
    sort: (SORT_KEYS as readonly string[]).includes(sort)
      ? (sort as SortKey)
      : "relevance",
    page: positiveInt(first(raw.page)) ?? 1,
  };
}

export type SearchChanges = {
  [K in keyof SearchState]?: SearchState[K] | undefined;
};

/**
 * `/search?...` for `state` with `changes` applied. Defaults (relevance sort,
 * page 1, empty values) are omitted. Changing anything but `page` resets the
 * page to 1, so a filter or sort change never lands on a stale page.
 */
export function buildSearchHref(
  state: SearchState,
  changes: SearchChanges = {},
): string {
  const next: SearchState = { ...state };
  for (const key of Object.keys(changes) as (keyof SearchState)[]) {
    (next as unknown as Record<string, unknown>)[key] = changes[key];
  }
  if (!("page" in changes)) next.page = 1;

  const params = new URLSearchParams();
  if (next.q) params.set("q", next.q);
  if (next.category) params.set("category", next.category);
  if (next.seller) params.set("seller", next.seller);
  if (next.rating) params.set("rating", next.rating);
  if (next.minPrice) params.set("minPrice", String(next.minPrice));
  if (next.maxPrice) params.set("maxPrice", String(next.maxPrice));
  if (next.sort && next.sort !== "relevance") params.set("sort", next.sort);
  if (next.page > 1) params.set("page", String(next.page));
  const qs = params.toString();
  return qs ? `/search?${qs}` : "/search";
}

/** How many filter groups are active (price counts once). Keyword and sort are not filters. */
export function activeFilterCount(state: SearchState): number {
  return (
    (state.category ? 1 : 0) +
    (state.seller ? 1 : 0) +
    (state.rating ? 1 : 0) +
    (state.minPrice || state.maxPrice ? 1 : 0)
  );
}

/** The filters (not keyword) are all cleared; `q` is kept when present. */
export function clearFiltersHref(state: SearchState): string {
  return buildSearchHref(state, {
    category: "",
    seller: "",
    rating: "",
    minPrice: undefined,
    maxPrice: undefined,
  });
}
