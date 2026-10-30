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
export default async function SearchPage(props: {
  searchParams: Promise<RawSearchParams>;
}) {
  const searchParams = await props.searchParams;
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
    <section className="space-y-6 py-4 md:space-y-8 md:py-8">
      <header className="space-y-3">
        <Breadcrumb items={trail} />
        <h1 className="text-2xl font-semibold tracking-tight text-text-primary md:text-3xl">
          {title}
        </h1>
        <Suspense
          fallback={
            <span className="block h-5 w-32 animate-pulse rounded-lg bg-surface-muted" />
          }
        >
          <ResultCount state={state} />
        </Suspense>
      </header>

      <CategoryBar
        categories={categories}
        selectedId={state.category || undefined}
        variant="pills"
      />

      <div className="flex flex-col gap-6 lg:flex-row lg:gap-8">
        <div className="w-full space-y-4 lg:sticky lg:top-36 lg:w-72 lg:shrink-0 lg:self-start">
          <SavedSearches currentQuery={state.q} initialSaved={savedSearches} />
          <Suspense fallback={<FilterSkeleton />}>
            <FilterPanel state={state} />
          </Suspense>
        </div>

        <div className="min-w-0 flex-1">
          <Suspense fallback={<ResultsSkeleton />}>
            <SearchResultsBlock state={state} />
          </Suspense>
        </div>
      </div>
    </section>
  );
}
