import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ActiveFilters } from "./ActiveFilters";
import { parseSearchParams } from "./url";

describe("ActiveFilters dynamic facets", () => {
  it("shows one removable chip per selected tag value and a clear-all link", () => {
    const state = parseSearchParams({
      q: "tai nghe",
      "sku.color": "xanh-navy,den",
      "tag.connectivity": "bluetooth-5-3",
    });
    render(<ActiveFilters state={state} />);
    const remove = screen.getByRole("link", {
      name: "Bỏ lọc Màu sắc: Xanh navy",
    });
    expect(remove).toHaveAttribute(
      "href",
      "/search?q=tai+nghe&sku.color=den&tag.connectivity=bluetooth-5-3",
    );
    expect(screen.getByText("Kết nối: Bluetooth 5 3")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Xóa tất cả bộ lọc" }),
    ).toHaveAttribute("href", "/search?q=tai+nghe");
  });
});

describe("ActiveFilters rating chip", () => {
  it("The rating chip removes the filter: a link named Từ 4 sao drops minRating", () => {
    render(
      <ActiveFilters state={parseSearchParams({ q: "ao", minRating: "4" })} />,
    );
    const chip = screen.getByRole("link", { name: /Từ [34] sao/ });
    expect(chip).toHaveAccessibleName(/Từ 4 sao/);
    expect(chip).toHaveAttribute("href", "/search?q=ao");
  });

  it("shows Từ 3 sao for minRating 3 and keeps the other chips on removal", () => {
    render(
      <ActiveFilters
        state={parseSearchParams({
          q: "ao",
          category: "c1",
          minRating: "3",
          page: "3",
        })}
      />,
    );
    const chip = screen.getByRole("link", { name: /Từ 3 sao/ });
    expect(chip).toHaveAttribute("href", "/search?q=ao&category=c1");
  });

  it("removing another chip keeps minRating, and clear-all drops it", () => {
    render(
      <ActiveFilters
        state={parseSearchParams({ q: "ao", category: "c1", minRating: "4" })}
      />,
    );
    const category = screen
      .getAllByRole("link", { name: /^Bỏ lọc/ })
      .find(
        (l) =>
          !/Từ 4 sao/.test(l.getAttribute("aria-label") ?? l.textContent ?? ""),
      );
    expect(category?.getAttribute("href")).toContain("minRating=4");
    expect(
      screen.getByRole("link", { name: "Xóa tất cả bộ lọc" }),
    ).toHaveAttribute("href", "/search?q=ao");
  });

  it("An unsupported minRating value is ignored: no chip", () => {
    render(
      <ActiveFilters state={parseSearchParams({ q: "ao", minRating: "9" })} />,
    );
    expect(screen.queryByRole("link", { name: /sao/ })).toBeNull();
  });

  it("An old rating link renders no rating chip", () => {
    render(
      <ActiveFilters state={parseSearchParams({ q: "ao", rating: "4" })} />,
    );
    expect(screen.queryByRole("link", { name: /sao/ })).toBeNull();
  });
});
