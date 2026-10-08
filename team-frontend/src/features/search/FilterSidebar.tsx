import Link from "next/link";

import { Card } from "@/components/ui/Card";
import { focusRing } from "@/components/ui/focus";
import type { ViewCategory } from "@/lib/gateway/listings";
import type { ViewFacets } from "@/lib/gateway/search";
import { FilterDrawer } from "./FilterDrawer";
import { PriceRangeForm } from "./PriceRangeForm";
import {
  type SearchState,
  activeFilterCount,
  buildSearchHref,
  parseSearchParams,
} from "./url";

function formatVnd(n: number): string {
  return `${n.toLocaleString("vi-VN")}₫`;
}

/**
 * A price-range facet key: "min-max" (closed) or "min+" / "min-" (open-ended, which is what
 * team-search emits for the top bucket, e.g. "1000000+").
 */
function parsePriceKey(key: string): { min: number; max: number | undefined } {
  const match = /^(\d+)(?:-(\d*)|\+)?$/.exec(key);
  if (!match) return { min: 0, max: undefined };
  return {
    min: Number(match[1]),
    max: match[2] ? Number(match[2]) : undefined,
  };
}

/** Human label for a price-range facet key like "0-100000" or "1000000+". */
function priceRangeLabel(key: string): string {
  const { min, max } = parsePriceKey(key);
  if (!max) return `Trên ${formatVnd(min)}`;
  if (min === 0) return `Dưới ${formatVnd(max)}`;
  return `${formatVnd(min)} - ${formatVnd(max)}`;
}

/** Parse a price-range facet key into the minPrice / maxPrice URL values. */
function priceRangeValues(key: string): {
  minPrice: number | undefined;
  maxPrice: number | undefined;
} {
  const { min, max } = parsePriceKey(key);
  return {
    minPrice: min > 0 ? min : undefined,
    maxPrice: max && max > 0 ? max : undefined,
  };
}

/** Canonical "min-max" key for the applied price filter (matches the facet keys). */
function currentPriceKey(min?: number, max?: number): string {
  return `${min ?? 0}-${max ?? ""}`;
}

/** Facet key in the same "min-max" shape as `currentPriceKey`. */
function normalizePriceKey(key: string): string {
  const { min, max } = parsePriceKey(key);
  return `${min}-${max ?? ""}`;
}

function Bucket({
  href,
  dataKey,
  label,
  count,
  active,
}: {
  href: string;
  dataKey: string;
  label: React.ReactNode;
  count: number;
  active: boolean;
}) {
  return (
    <Link
      href={href}
      data-testid="facet-bucket"
      data-key={dataKey}
      data-active={active ? "true" : "false"}
      aria-current={active ? "true" : undefined}
      className={`flex min-h-9 w-full items-center justify-between gap-2 rounded-lg px-2 text-left text-sm transition duration-150 ${focusRing} ${
        active
          ? "bg-primary-50 font-semibold text-action-primary"
          : "text-text-primary hover:bg-surface-muted hover:text-action-primary"
      }`}
    >
      <span className="flex min-w-0 items-center gap-2">
        <span
          aria-hidden="true"
          className={`grid h-4 w-4 shrink-0 place-items-center rounded-xs border text-xs leading-none ${
            active
              ? "border-action-primary bg-action-primary text-text-inverse"
              : "border-border-strong bg-surface-card"
          }`}
        >
          {active ? "✓" : ""}
        </span>
        <span className="truncate">{label}</span>
      </span>
      <span className="shrink-0 text-xs text-text-secondary">({count})</span>
    </Link>
  );
}

function Group({
  title,
  testId,
  children,
}: {
  title: string;
  testId?: string;
  children: React.ReactNode;
}) {
  return (
    <section
      data-testid={testId}
      className="space-y-1.5 border-t border-border-subtle pt-3 first:border-t-0 first:pt-0"
    >
      <h3 className="text-sm font-semibold text-text-primary">{title}</h3>
      {children}
    </section>
  );
}

export interface FilterSidebarProps {
  facets: ViewFacets;
  categories: ViewCategory[];
  currentCategory?: string;
  currentSeller?: string;
  currentMinPrice?: number;
  currentMaxPrice?: number;
  /** Keyword and sort to carry through every filter link (additive). */
  currentQuery?: string;
  currentSort?: string;
  /** sellerId -> shop name for the seller facet (additive); falls back to the id. */
  sellerNames?: Record<string, string>;
}

function FilterContent({
  facets,
  categories,
  state,
  sellerNames,
  idPrefix,
  showSubmit,
}: {
  facets: ViewFacets;
  categories: ViewCategory[];
  state: SearchState;
  sellerNames: Record<string, string>;
  idPrefix: string;
  showSubmit: boolean;
}) {
  const href = (changes: Parameters<typeof buildSearchHref>[1]) =>
    buildSearchHref(state, changes);
  const categoryName = (id: string) =>
    categories.find((c) => c.id === id)?.name || id;
  const priceKey = currentPriceKey(state.minPrice, state.maxPrice);

  const hidden: Record<string, string> = {};
  if (state.q) hidden.q = state.q;
  if (state.category) hidden.category = state.category;
  if (state.seller) hidden.seller = state.seller;
  if (state.sort !== "relevance") hidden.sort = state.sort;

  return (
    <div className="space-y-4">
      {facets.categories.length > 0 && (
        <Group title="Danh mục" testId="facet-categories">
          <div className="max-h-64 space-y-0.5 overflow-y-auto">
            {facets.categories.map((b) => {
              const active = state.category === b.key;
              return (
                <Bucket
                  key={b.key}
                  href={href({ category: active ? "" : b.key })}
                  dataKey={b.key}
                  label={categoryName(b.key)}
                  count={b.count}
                  active={active}
                />
              );
            })}
          </div>
        </Group>
      )}

      {facets.priceRanges.length > 0 && (
        <Group title="Khoảng giá" testId="facet-price_ranges">
          <div className="space-y-0.5">
            {facets.priceRanges.map((b) => {
              const active = priceKey === normalizePriceKey(b.key);
              return (
                <Bucket
                  key={b.key}
                  href={href(
                    active
                      ? { minPrice: undefined, maxPrice: undefined }
                      : priceRangeValues(b.key),
                  )}
                  dataKey={b.key}
                  label={priceRangeLabel(b.key)}
                  count={b.count}
                  active={active}
                />
              );
            })}
          </div>
        </Group>
      )}

      <Group title="Tự nhập giá">
        <PriceRangeForm
          id={`${idPrefix}-price-form`}
          hidden={hidden}
          minPrice={state.minPrice}
          maxPrice={state.maxPrice}
          showSubmit={showSubmit}
        />
      </Group>

      {facets.sellers.length > 0 && (
        <Group title="Nơi bán" testId="facet-sellers">
          <div className="space-y-0.5">
            {facets.sellers.map((b) => {
              const active = state.seller === b.key;
              return (
                <Bucket
                  key={b.key}
                  href={href({ seller: active ? "" : b.key })}
                  dataKey={b.key}
                  label={sellerNames[b.key] ?? b.key}
                  count={b.count}
                  active={active}
                />
              );
            })}
          </div>
        </Group>
      )}
    </div>
  );
}

/**
 * Search filters: facet buckets as plain links (work without JavaScript, URL
 * state, counts beside each bucket) plus a custom price-range form. From 1024px
 * up it is an inline card; below it hides behind a "Bộ lọc" button + Drawer.
 */
export function FilterSidebar({
  facets,
  categories,
  currentCategory,
  currentSeller,
  currentMinPrice,
  currentMaxPrice,
  currentQuery,
  currentSort,
  sellerNames = {},
}: FilterSidebarProps) {
  const state: SearchState = {
    ...parseSearchParams({
      q: currentQuery,
      category: currentCategory,
      seller: currentSeller,
      sort: currentSort,
    }),
    minPrice: currentMinPrice,
    maxPrice: currentMaxPrice,
  };
  const hasAnyFacet =
    facets.categories.length > 0 ||
    facets.priceRanges.length > 0 ||
    facets.sellers.length > 0;

  return (
    <aside data-testid="search-facets" className="w-full">
      <div className="hidden lg:block">
        <Card className="p-4">
          <FilterContent
            facets={facets}
            categories={categories}
            state={state}
            sellerNames={sellerNames}
            idPrefix="inline"
            showSubmit
          />
          {!hasAnyFacet && (
            <p className="mt-3 text-xs text-text-secondary">
              Chưa có bộ lọc nào cho kết quả này.
            </p>
          )}
        </Card>
      </div>

      <FilterDrawer
        activeCount={activeFilterCount(state)}
        formId="drawer-price-form"
      >
        <FilterContent
          facets={facets}
          categories={categories}
          state={state}
          sellerNames={sellerNames}
          idPrefix="drawer"
          showSubmit={false}
        />
      </FilterDrawer>
    </aside>
  );
}
