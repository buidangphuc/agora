import { render, screen, within } from "@testing-library/react";
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
    render(await SearchPage({ searchParams: { q: "ao", rating: "4" } }));
    const state = JSON.parse(
      screen.getByTestId("results-state").textContent ?? "",
    );
    expect(state).not.toHaveProperty("rating");
  });

  it("parses URL params into state for the results block and starts one search", async () => {
    render(
      await SearchPage({
        searchParams: { q: "ao", sort: "price_asc", page: "3" },
      }),
    );
    expect(
      JSON.parse(screen.getByTestId("results-state").textContent ?? ""),
    ).toEqual({
      q: "ao",
      category: "",
      seller: "",
      sort: "price_asc",
      page: 3,
    });
    expect(loadSearch).toHaveBeenCalledTimes(1);
    expect(screen.getByText("SAVED:ao")).toBeInTheDocument();
  });

  it("falls back to defaults for malformed params", async () => {
    render(await SearchPage({ searchParams: { page: "-2", sort: "sales" } }));
    const state = JSON.parse(
      screen.getByTestId("results-state").textContent ?? "",
    );
    expect(state.page).toBe(1);
    expect(state.sort).toBe("relevance");
  });

  it("uses marketplace copy, not the old room-rental text", async () => {
    const { container, rerender } = render(
      await SearchPage({ searchParams: {} }),
    );
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Tất cả sản phẩm",
    );
    rerender(await SearchPage({ searchParams: { q: "ao" } }));
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Kết quả cho “ao”",
    );
    expect(container.textContent).not.toMatch(/phòng|cho thuê/i);
  });

  it("shows the category in the breadcrumb and title", async () => {
    render(await SearchPage({ searchParams: { category: "c1" } }));
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
    render(await SearchPage({ searchParams: { category: "c1" } }));
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
