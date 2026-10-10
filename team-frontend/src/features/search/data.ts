import "server-only";

import { cache } from "react";

import { listCategories } from "@/lib/gateway/listings";
import {
  type FacetedSearchResult,
  SortBy,
  searchListings,
} from "@/lib/gateway/search";
import type { SearchState, SortKey } from "./url";

export type SearchLoad =
  | { ok: true; result: FacetedSearchResult }
  | { ok: false };

const SORT_BY: Record<SortKey, SortBy> = {
  relevance: SortBy.RELEVANCE,
  newest: SortBy.NEWEST,
  price_asc: SortBy.PRICE_ASC,
  price_desc: SortBy.PRICE_DESC,
};

/**
 * The search for one URL state, fetched once per request: the filter column,
 * the result count and the results column all read it from their own Suspense
 * boundary. A backend failure is a value (`ok: false`), never an empty result.
 */
export const loadSearch = cache(async (key: string): Promise<SearchLoad> => {
  const s: SearchState = JSON.parse(key);
  try {
    const result = await searchListings(s.q, {
      categoryId: s.category,
      sellerId: s.seller,
      minPrice: s.minPrice,
      maxPrice: s.maxPrice,
      sortBy: SORT_BY[s.sort],
      attrs: s.attrs,
      status: "published",
      page: s.page,
    });
    return { ok: true, result };
  } catch (err) {
    console.warn("search failed", err);
    return { ok: false };
  }
});

export function searchKey(state: SearchState): string {
  return JSON.stringify(state);
}

export const loadCategories = cache(async () =>
  listCategories().catch(() => []),
);
