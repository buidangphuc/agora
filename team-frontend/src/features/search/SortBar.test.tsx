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
  it("offers only the sorts the server honours", () => {
    const { container } = render(<SortBar totalResults={5} />);
    expect(container.textContent).not.toMatch(/bán chạy/i);
    const options = screen
      .getAllByRole("option")
      .map((o) => o.textContent)
      .filter(Boolean);
    expect(options).toEqual(
      expect.arrayContaining([
        "Liên quan",
        "Mới nhất",
        "Giá thấp đến cao",
        "Giá cao đến thấp",
      ]),
    );
    expect(screen.getByRole("link", { name: "Liên quan" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Mới nhất" })).toBeInTheDocument();
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
    expect(within(mobile).getAllByRole("option")).toHaveLength(4);
    expect(screen.queryByRole("link", { current: "page" })).toBeNull();
  });

  it("shows the result count", () => {
    render(<SortBar totalResults={42} />);
    expect(screen.getByText("42")).toBeInTheDocument();
  });
});
