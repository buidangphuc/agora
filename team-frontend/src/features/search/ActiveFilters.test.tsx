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
