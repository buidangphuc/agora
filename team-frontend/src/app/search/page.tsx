import Link from "next/link";

import { ListingGrid } from "@/features/listing/ListingGrid";
import { FilterSidebar } from "@/features/search/FilterSidebar";
import { SavedSearches } from "@/features/search/SavedSearches";
import { SortBar } from "@/features/search/SortBar";
import { SearchImpressions } from "@/features/tracking/SearchImpressions";
import { SortBy } from "@/generated/platform/search/v1/search_pb.js";
import { listCategories } from "@/lib/gateway/listings";
import {
  EMPTY_FACETS,
  type FacetedSearchResult,
  listSavedSearches,
  searchListings,
} from "@/lib/gateway/search";

export const dynamic = "force-dynamic";

export default async function SearchPage({
  searchParams,
}: {
  searchParams: {
    q?: string;
    category?: string;
    seller?: string;
    rating?: string;
    minPrice?: string;
    maxPrice?: string;
    sort?: string;
  };
}) {
  const q = searchParams.q ?? "";
  const categoryId = searchParams.category ?? "";
  const sellerId = searchParams.seller ?? "";
  const rating = searchParams.rating ?? "";
  const minRating = rating ? Number(rating) : undefined;
  const minPrice = searchParams.minPrice
    ? Number(searchParams.minPrice)
    : undefined;
  const maxPrice = searchParams.maxPrice
    ? Number(searchParams.maxPrice)
    : undefined;
  const sort = searchParams.sort ?? "relevance";

  let sortBy = SortBy.RELEVANCE;
  if (sort === "price_asc") sortBy = SortBy.PRICE_ASC;
  else if (sort === "price_desc") sortBy = SortBy.PRICE_DESC;
  else if (sort === "newest") sortBy = SortBy.NEWEST;

  const [categories, result, savedSearches] = await Promise.all([
    listCategories(),
    searchListings(q, {
      categoryId,
      sellerId,
      minPrice,
      maxPrice,
      minRating,
      sortBy,
      status: "published",
    }).catch(
      (): FacetedSearchResult => ({
        items: [],
        total: 0,
        facets: EMPTY_FACETS,
      }),
    ),
    listSavedSearches(),
  ]);

  const selectedCategory = categories.find((c) => c.id === categoryId);
  const hasActiveFilter =
    Boolean(selectedCategory) ||
    Boolean(sellerId) ||
    Boolean(rating) ||
    Boolean(minPrice) ||
    Boolean(maxPrice) ||
    Boolean(q);

  return (
    <section className="py-2">
      {/* ── Breadcrumb ── */}
      <nav className="mb-4 flex items-center gap-2 text-xs text-gray-500">
        <Link href="/" className="hover:text-emerald-700">
          Trang chủ
        </Link>
        <span>&gt;</span>
        {selectedCategory ? (
          <>
            <Link href="/search" className="hover:text-emerald-700">
              Tất cả danh mục cho thuê
            </Link>
            <span>&gt;</span>
            <span className="font-semibold text-gray-800">
              {selectedCategory.name}
            </span>
          </>
        ) : q ? (
          <span className="font-semibold text-gray-800">
            Kết quả tìm phòng cho &ldquo;{q}&rdquo;
          </span>
        ) : (
          <span className="font-semibold text-gray-800">
            Tất cả phòng cho thuê
          </span>
        )}
      </nav>

      {/* ── 2-Column Rental Layout ── */}
      <div className="flex flex-col gap-6 lg:flex-row">
        {/* Left: Saved searches + Facet Filter Sidebar */}
        <div className="w-full lg:w-64 lg:shrink-0 space-y-4">
          <SavedSearches currentQuery={q} initialSaved={savedSearches} />
          <FilterSidebar
            facets={result.facets}
            categories={categories}
            currentCategory={categoryId}
            currentSeller={sellerId}
            currentRating={rating}
            currentMinPrice={minPrice}
            currentMaxPrice={maxPrice}
          />
        </div>

        {/* Right: Results Area */}
        <div className="flex-1 min-w-0 space-y-4">
          {/* Header & Sort Bar */}
          <SortBar currentSort={sort} totalResults={result.total} />

          {/* Active Filter Badges */}
          {hasActiveFilter && (
            <div className="flex flex-wrap items-center gap-2 text-xs">
              <span className="text-gray-500">Đang lọc theo:</span>
              {q && (
                <span className="rounded-md bg-emerald-50 px-2 py-0.5 font-medium text-emerald-800 border border-emerald-200">
                  Từ khóa: &ldquo;{q}&rdquo;
                </span>
              )}
              {selectedCategory && (
                <span className="rounded-md bg-emerald-50 px-2 py-0.5 font-medium text-emerald-800 border border-emerald-200">
                  Loại phòng: {selectedCategory.name}
                </span>
              )}
              {sellerId && (
                <span className="rounded-md bg-emerald-50 px-2 py-0.5 font-medium text-emerald-800 border border-emerald-200">
                  Nơi bán: {sellerId}
                </span>
              )}
              {rating && (
                <span className="rounded-md bg-emerald-50 px-2 py-0.5 font-medium text-emerald-800 border border-emerald-200">
                  Đánh giá: từ {rating} sao
                </span>
              )}
              {(minPrice || maxPrice) && (
                <span className="rounded-md bg-emerald-50 px-2 py-0.5 font-medium text-emerald-800 border border-emerald-200">
                  Giá thuê: {minPrice ? minPrice.toLocaleString() : "0"}đ -{" "}
                  {maxPrice ? maxPrice.toLocaleString() : "∞"}đ
                </span>
              )}
              <Link
                href="/search"
                className="text-[11px] text-gray-400 underline hover:text-emerald-700 ml-2"
              >
                Xóa tất cả bộ lọc
              </Link>
            </div>
          )}

          {/* Fire IMPRESSION beacons for the rendered results (with position). */}
          <SearchImpressions
            listingIds={result.items.map((l) => l.id)}
            query={q}
          />

          {/* Listings Grid */}
          <div data-testid="search-results">
            <ListingGrid
              listings={result.items}
              empty="Không tìm thấy phòng cho thuê nào khớp với bộ lọc của bạn."
            />
          </div>
        </div>
      </div>
    </section>
  );
}
