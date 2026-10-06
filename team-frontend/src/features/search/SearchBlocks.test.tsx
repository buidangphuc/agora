import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ViewListing } from "@/lib/gateway/listings";
import type { FacetedSearchResult } from "@/lib/gateway/search";

const loadSearch = vi.hoisted(() => vi.fn());
const loadCategories = vi.hoisted(() => vi.fn());
const impressions = vi.hoisted(() => vi.fn());
const redirect = vi.hoisted(() => vi.fn());

vi.mock("./data", () => ({
  loadSearch,
  loadCategories,
  searchKey: (s: unknown) => JSON.stringify(s),
}));
vi.mock("next/navigation", () => ({
  redirect,
  notFound: vi.fn(),
  useRouter: () => ({ push: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/lib/gateway/search", () => ({
  EMPTY_FACETS: { categories: [], priceRanges: [], ratings: [], sellers: [] },
}));
vi.mock("@/lib/gateway/shops", () => ({
  batchGetShopNames: async () => new Map<string, string>(),
  shopLabel: (id: string, name?: string) => name || `Shop #${id.slice(0, 6)}`,
}));
vi.mock("@/features/tracking/SearchImpressions", () => ({
  SearchImpressions: (props: { listingIds: string[]; query?: string }) => {
    impressions(props);
    return null;
  },
}));
vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));
vi.mock("@/features/engagement/FavoriteButton", () => ({
  FavoriteButton: () => null,
}));

import { FilterPanel, ResultCount, SearchResultsBlock } from "./SearchBlocks";
import { type SearchState, parseSearchParams } from "./url";

function listing(id: string): ViewListing {
  return {
    id,
    title: `SP ${id}`,
    description: "",
    price: 100000,
    currency: "VND",
    status: "published",
    sellerId: "s",
    imageKeys: [],
    imageUrl: "https://img.test/x.jpg",
    categoryId: "c",
    stock: 1,
    variants: [],
  };
}

function ok(over: Partial<FacetedSearchResult> = {}) {
  return {
    ok: true,
    result: {
      items: [listing("a"), listing("b")],
      total: 60,
      page: 1,
      pageSize: 24,
      facets: { categories: [], priceRanges: [], ratings: [], sellers: [] },
      ...over,
    } satisfies FacetedSearchResult,
  };
}

const cats = [
  {
    id: "c1",
    name: "Điện thoại",
    iconUrl: "",
    slug: "",
    parentId: "",
    displayOrder: 0,
  },
];

function state(raw: Record<string, string>): SearchState {
  return parseSearchParams(raw);
}

beforeEach(() => {
  vi.clearAllMocks();
  loadCategories.mockResolvedValue(cats);
});

describe("SearchResultsBlock", () => {
  it("renders results, one impression, and pagination links that keep the query", async () => {
    loadSearch.mockResolvedValue(ok());
    const { container } = render(
      await SearchResultsBlock({ state: state({ q: "ao", sort: "newest" }) }),
    );
    expect(screen.getByTestId("search-results")).toBeInTheDocument();
    expect(impressions).toHaveBeenCalledTimes(1);
    expect(impressions).toHaveBeenCalledWith({
      listingIds: ["a", "b"],
      query: "ao",
    });
    const nav = container.querySelector("nav.hidden") as HTMLElement;
    const p2 = within(nav).getByRole("link", { name: "Trang 2" });
    expect(p2).toHaveAttribute("href", "/search?q=ao&sort=newest&page=2");
    expect(within(nav).getByRole("link", { name: "Trang 1" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(
      within(nav).getByRole("link", { name: "Trang 3" }),
    ).toBeInTheDocument();
  });

  it("shows only previous, current and next in the compact pager (375px)", async () => {
    loadSearch.mockResolvedValue(ok({ page: 2 }));
    const { container } = render(
      await SearchResultsBlock({ state: state({ q: "ao", page: "2" }) }),
    );
    const compact = container.querySelector("nav.sm\\:hidden") as HTMLElement;
    expect(within(compact).getAllByRole("listitem")).toHaveLength(3);
    expect(within(compact).getByLabelText("Trang trước")).toHaveAttribute(
      "href",
      "/search?q=ao",
    );
    expect(within(compact).getByLabelText("Trang sau")).toHaveAttribute(
      "href",
      "/search?q=ao&page=3",
    );
  });

  it("hides Pagination when the total fits one page", async () => {
    loadSearch.mockResolvedValue(ok({ total: 24 }));
    render(await SearchResultsBlock({ state: state({ q: "ao" }) }));
    expect(screen.queryByRole("navigation", { name: "Phân trang" })).toBeNull();
  });

  it("redirects an out-of-range page to the last page", async () => {
    loadSearch.mockResolvedValue(ok({ page: 3 }));
    await SearchResultsBlock({ state: state({ q: "ao", page: "99" }) });
    expect(redirect).toHaveBeenCalledWith("/search?q=ao&page=3");
  });

  it("redirects to the first page without a page param", async () => {
    loadSearch.mockResolvedValue(ok({ page: 1, items: [], total: 0 }));
    await SearchResultsBlock({ state: state({ q: "ao", page: "4" }) });
    expect(redirect).toHaveBeenCalledWith("/search?q=ao");
  });

  it("shows Empty with a clear-filters action on zero results", async () => {
    loadSearch.mockResolvedValue(ok({ items: [], total: 0 }));
    render(await SearchResultsBlock({ state: state({ q: "zzzzzz" }) }));
    expect(screen.getByText("Không tìm thấy sản phẩm")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Xóa bộ lọc" })).toHaveAttribute(
      "href",
      "/search",
    );
    expect(screen.getByTestId("search-results")).toBeInTheDocument();
  });

  it("shows an error Alert with retry, not 'no results', when the search fails", async () => {
    loadSearch.mockResolvedValue({ ok: false });
    render(
      await SearchResultsBlock({ state: state({ q: "ao", sort: "newest" }) }),
    );
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Thử lại" })).toHaveAttribute(
      "href",
      "/search?q=ao&sort=newest",
    );
    expect(screen.queryByText("Không tìm thấy sản phẩm")).toBeNull();
    expect(screen.queryByTestId("search-results")).toBeNull();
    expect(impressions).not.toHaveBeenCalled();
  });

  it("does not offer the unwired 'Bán chạy' sort", async () => {
    loadSearch.mockResolvedValue(ok());
    const { container } = render(
      await SearchResultsBlock({ state: state({ q: "ao" }) }),
    );
    expect(container.textContent).not.toMatch(/bán chạy/i);
  });

  it("lists active filters as removable tags and clears them all", async () => {
    loadSearch.mockResolvedValue(ok());
    render(
      await SearchResultsBlock({
        state: state({ q: "ao", category: "c1", rating: "4" }),
      }),
    );
    const tags = screen.getByTestId("active-filters");
    expect(within(tags).getAllByRole("link", { name: /^Bỏ lọc/ })).toHaveLength(
      3,
    );
    expect(
      within(tags).getByRole("link", { name: "Bỏ lọc Danh mục: Điện thoại" }),
    ).toHaveAttribute("href", "/search?q=ao&rating=4");
    expect(
      within(tags).getByRole("link", { name: "Xóa tất cả bộ lọc" }),
    ).toHaveAttribute("href", "/search?q=ao");
  });
});

describe("ResultCount / FilterPanel", () => {
  it("shows the count and hides it on failure", async () => {
    loadSearch.mockResolvedValue(ok({ total: 7 }));
    const { unmount } = render(await ResultCount({ state: state({}) }));
    expect(screen.getByText("7")).toBeInTheDocument();
    unmount();
    loadSearch.mockResolvedValue({ ok: false });
    expect(await ResultCount({ state: state({}) })).toBeNull();
  });

  it("keeps the filter column when the search fails", async () => {
    loadSearch.mockResolvedValue({ ok: false });
    render(await FilterPanel({ state: state({ q: "ao" }) }));
    expect(screen.getByTestId("search-facets")).toBeInTheDocument();
  });

  it("labels the seller facet with the shop name fallback, not the raw id", async () => {
    loadSearch.mockResolvedValue(
      ok({
        facets: {
          categories: [],
          priceRanges: [],
          ratings: [],
          sellers: [{ key: "abcdef123456", count: 2 }],
        },
      }),
    );
    render(await FilterPanel({ state: state({}) }));
    expect(
      screen.getAllByText("Shop #".concat("abcdef")).length,
    ).toBeGreaterThan(0);
  });
});
