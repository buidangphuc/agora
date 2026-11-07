import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
  redirect: vi.fn(),
  notFound: vi.fn(),
}));

import { SortBar } from "./SortBar";
import { parseSearchParams } from "./url";

describe("SortBar", () => {
  it("Sort options match what the server honours: Bán chạy is offered", () => {
    render(<SortBar totalResults={5} />);
    const options = screen
      .getAllByRole("option")
      .map((o) => o.textContent)
      .filter(Boolean);
    expect(options).toEqual(
      expect.arrayContaining([
        "Liên quan",
        "Mới nhất",
        "Bán chạy",
        "Giá thấp đến cao",
        "Giá cao đến thấp",
      ]),
    );
    expect(screen.getByRole("link", { name: "Liên quan" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Mới nhất" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Bán chạy" })).toHaveAttribute(
      "href",
      "/search?sort=best_selling",
    );
  });

  it("Sort options match what the server honours: Bán chạy link keeps the query, resets the page and keeps minRating", () => {
    render(
      <SortBar
        totalResults={5}
        state={parseSearchParams({ q: "ao", page: "3", minRating: "4" })}
      />,
    );
    const href = screen
      .getByRole("link", { name: "Bán chạy" })
      .getAttribute("href");
    const p = new URL(`http://x${href}`).searchParams;
    expect(p.get("sort")).toBe("best_selling");
    expect(p.get("q")).toBe("ao");
    expect(p.get("minRating")).toBe("4");
    expect(p.get("page")).toBeNull();
    expect(href).not.toContain("rating=4&");
  });

  it("Sort options match what the server honours: the active Bán chạy is marked", () => {
    render(
      <SortBar
        currentSort="best_selling"
        state={parseSearchParams({ sort: "best_selling" })}
      />,
    );
    expect(screen.getByRole("link", { name: "Bán chạy" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: "Liên quan" })).not.toHaveAttribute(
      "aria-current",
    );
    const mobile = screen.getByLabelText("Sắp xếp") as HTMLSelectElement;
    expect(mobile.value).toBe("best_selling");
  });

  it("An unknown sort value falls back to relevance", () => {
    render(
      <SortBar
        currentSort="top_rated"
        state={parseSearchParams({ sort: "top_rated" })}
      />,
    );
    expect(screen.getByRole("link", { name: "Liên quan" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: "Bán chạy" })).not.toHaveAttribute(
      "aria-current",
    );
  });

  it("marks the active tab and links keep the query, resetting the page", () => {
    render(
      <SortBar
        currentSort="newest"
        totalResults={5}
        state={parseSearchParams({ q: "ao", sort: "newest", page: "3" })}
      />,
    );
    expect(screen.getByRole("link", { name: "Mới nhất" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: "Liên quan" })).not.toHaveAttribute(
      "aria-current",
    );
    expect(screen.getByRole("link", { name: "Liên quan" })).toHaveAttribute(
      "href",
      "/search?q=ao",
    );
  });

  it("selects the price option in the select when a price sort is active", () => {
    render(
      <SortBar
        currentSort="price_asc"
        state={parseSearchParams({ sort: "price_asc" })}
      />,
    );
    const mobile = screen.getByLabelText("Sắp xếp") as HTMLSelectElement;
    expect(mobile.value).toBe("price_asc");
    expect(within(mobile).getAllByRole("option")).toHaveLength(5);
    expect(screen.queryByRole("link", { current: "page" })).toBeNull();
  });

  it("shows the result count", () => {
    render(<SortBar totalResults={42} />);
    expect(screen.getByText("42")).toBeInTheDocument();
  });
});
