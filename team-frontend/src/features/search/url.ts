/**
 * URL state of /search (UI_SYSTEM_DESIGN.md section 5.B): `q, category, seller,
 * minPrice, maxPrice, sort, page`, plus one `tag.<group>` / `sku.<group>` param per
 * dynamic facet group (comma-separated tag slugs; OR inside a group, AND across). Pure helpers (no React, no gateway)
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
  minPrice?: number;
  maxPrice?: number;
  sort: SortKey;
  /**
   * Dynamic facet selections keyed by the request filter key (`tag.connectivity`,
   * `sku.color`) -> selected tag slugs. Always present; empty when none.
   */
  attrs: Record<string, string[]>;
  /** 1-based. */
  page: number;
}

const ATTR_KEY = /^(tag|sku)\.[a-z][a-z0-9_]{0,31}$/;
const ATTR_SLUG = /^[a-z0-9][a-z0-9-]{0,63}$/;

function parseAttrs(raw: RawSearchParams): Record<string, string[]> {
  const out: Record<string, string[]> = {};
  for (const key of Object.keys(raw).sort()) {
    if (!ATTR_KEY.test(key)) continue;
    const slugs = first(raw[key])
      .split(",")
      .map((v) => v.trim())
      .filter((v) => ATTR_SLUG.test(v));
    const unique = [...new Set(slugs)];
    if (unique.length > 0) out[key] = unique;
  }
  return out;
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
  const sort = first(raw.sort);
  return {
    q: first(raw.q),
    category: first(raw.category),
    seller: first(raw.seller),
    minPrice: positiveInt(first(raw.minPrice)),
    maxPrice: positiveInt(first(raw.maxPrice)),
    sort: (SORT_KEYS as readonly string[]).includes(sort)
      ? (sort as SortKey)
      : "relevance",
    attrs: parseAttrs(raw),
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
  if (next.minPrice) params.set("minPrice", String(next.minPrice));
  if (next.maxPrice) params.set("maxPrice", String(next.maxPrice));
  for (const key of Object.keys(next.attrs ?? {}).sort()) {
    const slugs = next.attrs[key];
    if (slugs && slugs.length > 0) params.set(key, slugs.join(","));
  }
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
    (state.minPrice || state.maxPrice ? 1 : 0) +
    Object.values(state.attrs ?? {}).filter((v) => v.length > 0).length
  );
}

/** The filters (not keyword) are all cleared; `q` is kept when present. */
export function clearFiltersHref(state: SearchState): string {
  return buildSearchHref(state, {
    category: "",
    seller: "",
    minPrice: undefined,
    maxPrice: undefined,
    attrs: {},
  });
}

/** `attrs` with `slug` toggled in `key` (added when absent, removed when present). */
export function toggleAttr(
  attrs: Record<string, string[]>,
  key: string,
  slug: string,
): Record<string, string[]> {
  const current = attrs[key] ?? [];
  const next = current.includes(slug)
    ? current.filter((v) => v !== slug)
    : [...current, slug];
  const out = { ...attrs };
  if (next.length > 0) out[key] = next;
  else delete out[key];
  return out;
}
