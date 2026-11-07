import { cleanup, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const loadSearch = vi.hoisted(() => vi.fn());
const loadCategories = vi.hoisted(() => vi.fn());

vi.mock("@/features/search/data", () => ({
  loadSearch,
  loadCategories,
  searchKey: (s: unknown) => JSON.stringify(s),
}));
vi.mock("@/features/search/SearchBlocks", () => ({
  ResultCount: () => <span>COUNT</span>,
  FilterPanel: () => <div>FILTERS</div>,
  SearchResultsBlock: ({ state }: { state: unknown }) => (
    <div data-testid="results-state">{JSON.stringify(state)}</div>
  ),
}));
vi.mock("@/features/search/SavedSearches", () => ({
  SavedSearches: ({ currentQuery }: { currentQuery: string }) => (
    <div>SAVED:{currentQuery}</div>
  ),
}));
vi.mock("@/lib/gateway/search", () => ({
  listSavedSearches: async () => [],
}));

import SearchPage from "./page";

beforeEach(() => {
  vi.clearAllMocks();
  loadSearch.mockResolvedValue({ ok: false });
  loadCategories.mockResolvedValue([
    {
      id: "c1",
      name: "Điện thoại",
      iconUrl: "",
      slug: "",
      parentId: "",
      displayOrder: 0,
    },
  ]);
});

describe("SearchPage", () => {
  it("ignores an old rating param", async () => {
    render(
      await SearchPage({
        searchParams: Promise.resolve({ q: "ao", rating: "4" }),
      }),
    );
    const state = JSON.parse(
      screen.getByTestId("results-state").textContent ?? "",
    );
    expect(state).not.toHaveProperty("rating");
  });

  const stateFor = async (params: Record<string, string | string[]>) => {
    render(await SearchPage({ searchParams: Promise.resolve(params) }));
    return JSON.parse(screen.getByTestId("results-state").textContent ?? "");
  };

  it("An old rating link renders the results without error: same state as without rating", async () => {
    const withRating = await stateFor({ q: "ao", rating: "4" });
    cleanup();
    expect(withRating).toEqual(await stateFor({ q: "ao" }));
    expect(withRating.minRating).toBeUndefined();
  });

  it("Choosing a rating filters the results by URL: minRating=4 and 3 reach the state", async () => {
    expect((await stateFor({ q: "ao", minRating: "4" })).minRating).toBe(4);
    cleanup();
    expect((await stateFor({ q: "ao", minRating: "3" })).minRating).toBe(3);
  });

  it("An unsupported minRating value is ignored", async () => {
    for (const v of ["9", "abc"]) {
      const state = await stateFor({ q: "ao", minRating: v });
      cleanup();
      expect(state.minRating).toBeUndefined();
    }
  });

  it("A repeated minRating uses the first value", async () => {
    expect((await stateFor({ q: "ao", minRating: ["4", "3"] })).minRating).toBe(
      4,
    );
  });

  it("Best-selling sort: sort=best_selling reaches the state", async () => {
    expect((await stateFor({ q: "ao", sort: "best_selling" })).sort).toBe(
      "best_selling",
    );
  });

  it("An unknown sort value falls back to relevance", async () => {
    expect((await stateFor({ q: "ao", sort: "top_rated" })).sort).toBe(
      "relevance",
    );
  });

  it("parses URL params into state for the results block and starts one search", async () => {
    render(
      await SearchPage({
        searchParams: Promise.resolve({
          q: "ao",
          sort: "price_asc",
          page: "3",
        }),
      }),
    );
    expect(
      JSON.parse(screen.getByTestId("results-state").textContent ?? ""),
    ).toEqual({
      q: "ao",
      category: "",
      seller: "",
      sort: "price_asc",
      attrs: {},
      page: 3,
    });
    expect(loadSearch).toHaveBeenCalledTimes(1);
    expect(screen.getByText("SAVED:ao")).toBeInTheDocument();
  });

  it("falls back to defaults for malformed params", async () => {
    render(
      await SearchPage({
        searchParams: Promise.resolve({ page: "-2", sort: "sales" }),
      }),
    );
    const state = JSON.parse(
      screen.getByTestId("results-state").textContent ?? "",
    );
    expect(state.page).toBe(1);
    expect(state.sort).toBe("relevance");
  });

  it("uses marketplace copy, not the old room-rental text", async () => {
    const { container, rerender } = render(
      await SearchPage({ searchParams: Promise.resolve({}) }),
    );
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Tất cả sản phẩm",
    );
    rerender(await SearchPage({ searchParams: Promise.resolve({ q: "ao" }) }));
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Kết quả cho “ao”",
    );
    expect(container.textContent).not.toMatch(/phòng|cho thuê/i);
  });

  it("shows the category in the breadcrumb and title", async () => {
    render(
      await SearchPage({ searchParams: Promise.resolve({ category: "c1" }) }),
    );
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Điện thoại",
    );
    expect(
      screen.getByRole("link", { name: "Tất cả sản phẩm" }),
    ).toHaveAttribute("href", "/search");
    expect(screen.getByRole("link", { name: "Trang chủ" })).toHaveAttribute(
      "href",
      "/",
    );
  });

  it("mounts category pills with the current category marked", async () => {
    render(
      await SearchPage({ searchParams: Promise.resolve({ category: "c1" }) }),
    );
    const nav = screen.getByRole("navigation", { name: "Danh mục sản phẩm" });
    expect(
      within(nav).getByRole("link", { name: /Điện thoại/ }),
    ).toHaveAttribute("aria-current", "true");
    expect(
      within(nav).getByRole("link", { name: "Tất cả" }),
    ).not.toHaveAttribute("aria-current");
    expect(
      within(nav).getByRole("link", { name: /Điện thoại/ }),
    ).toHaveAttribute("href", "/search?category=c1");
  });
});
