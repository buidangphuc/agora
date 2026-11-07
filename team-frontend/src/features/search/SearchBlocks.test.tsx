import { render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ViewListing } from "@/lib/gateway/listings";
import type { FacetedSearchResult } from "@/lib/gateway/search";
import {
  beginBeaconCapture,
  drain,
  endBeaconCapture,
  listEntries,
} from "@/test/listBeacons";

const loadSearch = vi.hoisted(() => vi.fn());
const loadCategories = vi.hoisted(() => vi.fn());
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
  EMPTY_FACETS: {
    categories: [],
    priceRanges: [],
    ratings: [],
    sellers: [],
    tags: [],
    skus: [],
  },
}));
vi.mock("@/lib/gateway/shops", () => ({
  batchGetShopNames: async () => new Map<string, string>(),
  shopLabel: (id: string, name?: string) => name || `Shop #${id.slice(0, 6)}`,
}));
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
      facets: {
        categories: [],
        priceRanges: [],
        ratings: [],
        sellers: [],
        tags: [],
        skus: [],
      },
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

function state(raw: Record<string, string | string[]>): SearchState {
  return parseSearchParams(raw);
}

afterEach(endBeaconCapture);

beforeEach(() => {
  vi.clearAllMocks();
  loadCategories.mockResolvedValue(cats);
});

describe("SearchResultsBlock", () => {
  it("renders results, one impression, and pagination links that keep the query", async () => {
    beginBeaconCapture();
    loadSearch.mockResolvedValue(ok());
    const { container } = render(
      await SearchResultsBlock({ state: state({ q: "ao", sort: "newest" }) }),
    );
    expect(screen.getByTestId("search-results")).toBeInTheDocument();
    await drain();
    const entries = listEntries();
    expect(entries).toHaveLength(1);
    expect(entries[0].ecommerce?.items?.map((i) => i.item_list_id)).toEqual([
      "search_results",
      "search_results",
    ]);
    const nav = container.querySelector(
      ".hidden.justify-center > nav",
    ) as HTMLElement;
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
    beginBeaconCapture();
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
    await drain();
    expect(listEntries()).toHaveLength(0);
  });

  it("Sort options match what the server honours: offers Bán chạy linking to sort=best_selling", async () => {
    loadSearch.mockResolvedValue(ok());
    render(await SearchResultsBlock({ state: state({ q: "ao" }) }));
    expect(screen.getByRole("link", { name: "Bán chạy" })).toHaveAttribute(
      "href",
      "/search?q=ao&sort=best_selling",
    );
  });

  it("Best-selling sort: marks Bán chạy active for sort=best_selling", async () => {
    loadSearch.mockResolvedValue(ok());
    render(
      await SearchResultsBlock({
        state: state({ q: "ao", sort: "best_selling" }),
      }),
    );
    expect(screen.getByRole("link", { name: "Bán chạy" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("An unknown sort value falls back to relevance", async () => {
    loadSearch.mockResolvedValue(ok());
    render(
      await SearchResultsBlock({
        state: state({ q: "ao", sort: "top_rated" }),
      }),
    );
    expect(screen.getByRole("link", { name: "Liên quan" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("The rating chip removes the filter: shows one chip that links to the URL without minRating", async () => {
    loadSearch.mockResolvedValue(ok());
    render(
      await SearchResultsBlock({
        state: state({ q: "ao", minRating: "4", page: "2" }),
      }),
    );
    const tags = screen.getByTestId("active-filters");
    const chips = within(tags).getAllByRole("link", { name: /Từ [34] sao/ });
    expect(chips).toHaveLength(1);
    expect(chips[0]).toHaveAccessibleName(/Từ 4 sao/);
    expect(chips[0]).toHaveAttribute("href", "/search?q=ao");
  });

  it("A repeated minRating uses the first value: one chip for Từ 4 sao", async () => {
    loadSearch.mockResolvedValue(ok());
    render(
      await SearchResultsBlock({
        state: parseSearchParams({ q: "ao", minRating: ["4", "3"] }),
      }),
    );
    const chips = within(screen.getByTestId("active-filters")).getAllByRole(
      "link",
      { name: /Từ [34] sao/ },
    );
    expect(chips).toHaveLength(1);
    expect(chips[0]).toHaveAccessibleName(/Từ 4 sao/);
  });

  it("An unsupported minRating value is ignored: no rating chip", async () => {
    loadSearch.mockResolvedValue(ok());
    render(
      await SearchResultsBlock({
        state: parseSearchParams({ q: "ao", minRating: "9" }),
      }),
    );
    expect(screen.queryByRole("link", { name: /Từ [34] sao/ })).toBeNull();
  });

  it("An old rating link renders the results without error: no chip, no rating= link", async () => {
    loadSearch.mockResolvedValue(ok());
    const { container } = render(
      await SearchResultsBlock({
        state: parseSearchParams({ q: "ao", rating: "4" }),
      }),
    );
    expect(screen.getByTestId("search-results")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByRole("link", { name: /Từ [34] sao/ })).toBeNull();
    expect(container.innerHTML).not.toMatch(/[?&]rating=/);
  });

  it("A rating filter with no match shows the empty state with the rating chip", async () => {
    loadSearch.mockResolvedValue(ok({ items: [], total: 0 }));
    render(
      await SearchResultsBlock({ state: state({ q: "ao", minRating: "4" }) }),
    );
    expect(screen.getByText("Không tìm thấy sản phẩm")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.getByRole("link", { name: /Từ 4 sao/ })).toBeInTheDocument();
  });

  it("lists active filters as removable tags and clears them all", async () => {
    loadSearch.mockResolvedValue(ok());
    render(
      await SearchResultsBlock({
        state: state({ q: "ao", category: "c1", seller: "s1" }),
      }),
    );
    const tags = screen.getByTestId("active-filters");
    expect(within(tags).getAllByRole("link", { name: /^Bỏ lọc/ })).toHaveLength(
      3,
    );
    expect(
      within(tags).getByRole("link", { name: "Bỏ lọc Danh mục: Điện thoại" }),
    ).toHaveAttribute("href", "/search?q=ao&seller=s1");
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

  const ratingFacets = {
    categories: [],
    priceRanges: [],
    ratings: [
      { key: "rating_floor_4", count: 3 },
      { key: "rating_floor_3", count: 5 },
    ],
    sellers: [],
    tags: [],
    skus: [],
  };

  it("The search page shows the rating filter: facet-ratings with both options and counts from the response", async () => {
    loadSearch.mockResolvedValue(ok({ facets: ratingFacets }));
    const { container } = render(
      await FilterPanel({ state: state({ q: "ao" }) }),
    );
    const groups = screen.getAllByTestId("facet-ratings");
    expect(groups.length).toBeGreaterThan(0);
    const group = groups[0] as HTMLElement;
    expect(
      within(group)
        .getByText(/Từ 4 sao/)
        .closest("a")?.textContent,
    ).toContain("(3)");
    expect(
      within(group)
        .getByText(/Từ 3 sao/)
        .closest("a")?.textContent,
    ).toContain("(5)");
    expect(container.innerHTML).not.toMatch(/[?&]rating=/);
  });

  it("The rating group keeps both options while one is selected", async () => {
    loadSearch.mockResolvedValue(ok({ facets: ratingFacets }));
    render(await FilterPanel({ state: state({ q: "ao", minRating: "4" }) }));
    const group = screen.getAllByTestId("facet-ratings")[0] as HTMLElement;
    expect(
      within(group)
        .getByText(/Từ 4 sao/)
        .closest("a"),
    ).toHaveAttribute("data-active", "true");
    const three = within(group)
      .getByText(/Từ 3 sao/)
      .closest("a");
    expect(three).toHaveAttribute("data-active", "false");
    expect(three?.textContent).toContain("(5)");
  });

  it("shows no rating group when the response has no rating facets", async () => {
    loadSearch.mockResolvedValue(ok());
    render(await FilterPanel({ state: state({}) }));
    expect(screen.queryByTestId("facet-ratings")).toBeNull();
  });

  it("keeps the filter column when the search fails", async () => {
    beginBeaconCapture();
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
          tags: [],
          skus: [],
        },
      }),
    );
    render(await FilterPanel({ state: state({}) }));
    expect(
      screen.getAllByText("Shop #".concat("abcdef")).length,
    ).toBeGreaterThan(0);
  });
});
