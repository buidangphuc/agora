import Link from "next/link";
import { redirect } from "next/navigation";

import { Alert } from "@/components/ui/Alert";
import { Card } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Pagination } from "@/components/ui/Pagination";
import { ListingGrid } from "@/features/listing/ListingGrid";
import { SearchImpressions } from "@/features/tracking/SearchImpressions";
import { EMPTY_FACETS } from "@/lib/gateway/search";
import { batchGetShopNames, shopLabel } from "@/lib/gateway/shops";
import { ActiveFilters } from "./ActiveFilters";
import { CompactPagination } from "./CompactPagination";
import { FilterSidebar } from "./FilterSidebar";
import { SortBar } from "./SortBar";
import { loadCategories, loadSearch, searchKey } from "./data";
import { type SearchState, buildSearchHref } from "./url";

const linkButton =
  "inline-flex min-h-11 items-center justify-center rounded-lg bg-action-primary px-4 text-sm font-medium text-text-inverse shadow-sm transition duration-150 hover:bg-action-primary-hover active:scale-95";

/** "Tìm thấy N sản phẩm" under the page title. Renders nothing on failure. */
export async function ResultCount({ state }: { state: SearchState }) {
  const load = await loadSearch(searchKey(state));
  if (!load.ok) return null;
  return (
    <p className="text-sm text-text-secondary">
      Tìm thấy{" "}
      <strong className="font-semibold text-text-primary">
        {load.result.total}
      </strong>{" "}
      sản phẩm
    </p>
  );
}

/** Filter column. A failed search still shows the (empty) filters, never a crash. */
export async function FilterPanel({ state }: { state: SearchState }) {
  const [load, categories] = await Promise.all([
    loadSearch(searchKey(state)),
    loadCategories(),
  ]);
  const facets = load.ok ? load.result.facets : EMPTY_FACETS;

  const ids = facets.sellers.map((b) => b.key);
  if (state.seller && !ids.includes(state.seller)) ids.push(state.seller);
  const names = await batchGetShopNames(ids);
  const sellerNames = Object.fromEntries(
    ids.map((id) => [id, shopLabel(id, names.get(id))]),
  );

  return (
    <FilterSidebar
      facets={facets}
      categories={categories}
      currentCategory={state.category}
      currentSeller={state.seller}
      currentMinPrice={state.minPrice}
      currentMaxPrice={state.maxPrice}
      currentAttrs={state.attrs}
      currentQuery={state.q}
      currentSort={state.sort}
      sellerNames={sellerNames}
    />
  );
}

/**
 * Results column: sort, active filters, the grid (`data-testid="search-results"`)
 * and pagination. A page past the end redirects to the last page; a backend
 * failure is an error Alert with a retry link, never "no results".
 */
export async function SearchResultsBlock({ state }: { state: SearchState }) {
  const [load, categories] = await Promise.all([
    loadSearch(searchKey(state)),
    loadCategories(),
  ]);
  const categoryName = categories.find((c) => c.id === state.category)?.name;
  const sellerName = state.seller
    ? shopLabel(
        state.seller,
        (await batchGetShopNames([state.seller])).get(state.seller),
      )
    : undefined;

  const filters = (
    <ActiveFilters
      state={state}
      categoryName={categoryName}
      sellerName={sellerName}
    />
  );

  if (!load.ok) {
    return (
      <div className="space-y-4">
        {filters}
        <Alert
          type="error"
          title="Không tải được kết quả tìm kiếm"
          description="Hệ thống tìm kiếm tạm thời không phản hồi. Vui lòng thử lại."
          action={
            <Link
              href={buildSearchHref(state, { page: state.page })}
              className="text-sm font-medium text-action-primary underline"
            >
              Thử lại
            </Link>
          }
        />
      </div>
    );
  }

  const { result } = load;
  if (result.page !== state.page) {
    redirect(buildSearchHref(state, { page: result.page }));
  }

  const hrefFor = (page: number) => buildSearchHref(state, { page });

  return (
    <div className="space-y-4">
      <SortBar
        currentSort={state.sort}
        totalResults={result.total}
        state={state}
      />
      {filters}

      {/* One batched impression for the rendered results (position = index + 1). */}
      <SearchImpressions
        listingIds={result.items.map((l) => l.id)}
        query={state.q}
      />

      <div data-testid="search-results">
        {result.items.length === 0 ? (
          <Card>
            <Empty
              description={
                <>
                  <span className="block font-semibold text-text-primary">
                    Không tìm thấy sản phẩm
                  </span>
                  <span className="mt-1 block text-xs">
                    Hãy thử từ khóa khác hoặc bỏ bớt bộ lọc.
                  </span>
                </>
              }
              action={
                <Link href="/search" className={linkButton}>
                  Xóa bộ lọc
                </Link>
              }
            />
          </Card>
        ) : (
          <ListingGrid listings={result.items} />
        )}
      </div>

      <Pagination
        current={result.page}
        total={result.total}
        pageSize={result.pageSize}
        hrefFor={hrefFor}
        className="hidden justify-center sm:flex"
      />
      <CompactPagination
        current={result.page}
        total={result.total}
        pageSize={result.pageSize}
        hrefFor={hrefFor}
        className="sm:hidden"
      />
    </div>
  );
}
