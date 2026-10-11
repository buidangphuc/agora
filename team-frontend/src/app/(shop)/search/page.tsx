import { Suspense } from "react";

import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { CategoryBar } from "@/features/listing/CategoryBar";
import { SavedSearches } from "@/features/search/SavedSearches";
import {
  FilterPanel,
  ResultCount,
  SearchResultsBlock,
} from "@/features/search/SearchBlocks";
import {
  FilterSkeleton,
  ResultsSkeleton,
} from "@/features/search/SearchSkeletons";
import { loadCategories, loadSearch, searchKey } from "@/features/search/data";
import { type RawSearchParams, parseSearchParams } from "@/features/search/url";
import { listSavedSearches } from "@/lib/gateway/search";

export const dynamic = "force-dynamic";

/**
 * Search List anatomy: header (breadcrumb, title, count), filter column and
 * results column. All state is in the URL (`q, category, seller,
 * minPrice, maxPrice, sort, page`). The search runs once (cached per request)
 * and the filter column, count and results each stream from their own boundary.
 */
export default async function SearchPage({
  searchParams,
}: {
  searchParams: RawSearchParams;
}) {
  const state = parseSearchParams(searchParams);
  // Start the search now; the boundaries below read the same cached promise.
  void loadSearch(searchKey(state));
  const [categories, savedSearches] = await Promise.all([
    loadCategories(),
    listSavedSearches(),
  ]);

  const selectedCategory = categories.find((c) => c.id === state.category);
  const title = selectedCategory
    ? selectedCategory.name
    : state.q
      ? `Kết quả cho “${state.q}”`
      : "Tất cả sản phẩm";
  const trail = selectedCategory
    ? [
        { label: "Trang chủ", href: "/" },
        { label: "Tất cả sản phẩm", href: "/search" },
        { label: selectedCategory.name },
      ]
    : [{ label: "Trang chủ", href: "/" }, { label: title }];

  return (
    <section className="space-y-4 py-2">
      <header className="space-y-2">
        <Breadcrumb items={trail} />
        <h1 className="text-lg font-semibold text-text-primary">{title}</h1>
        <Suspense
          fallback={
            <span className="block h-5 w-32 animate-pulse rounded-xs bg-neutral-200" />
          }
        >
          <ResultCount state={state} />
        </Suspense>
      </header>

      {/* Category pills: plain links, rendered with the page so the strip
          never shifts layout after paint. */}
      <CategoryBar
        categories={categories}
        selectedId={state.category || undefined}
        variant="pills"
      />

      <div className="flex flex-col gap-6 lg:flex-row">
        {/* Filter column: saved searches + facet filters */}
        <div className="w-full space-y-4 lg:sticky lg:top-36 lg:w-64 lg:shrink-0 lg:self-start">
          <SavedSearches currentQuery={state.q} initialSaved={savedSearches} />
          <Suspense fallback={<FilterSkeleton />}>
            <FilterPanel state={state} />
          </Suspense>
        </div>

        {/* Results column */}
        <div className="min-w-0 flex-1">
          <Suspense fallback={<ResultsSkeleton />}>
            <SearchResultsBlock state={state} />
          </Suspense>
        </div>
      </div>
    </section>
  );
}
