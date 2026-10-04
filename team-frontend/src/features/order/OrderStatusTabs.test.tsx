import { render, screen } from "@testing-library/react";
import React from "react";
import { describe, expect, it } from "vitest";

import { OrderStatusTabs, ordersHref } from "./OrderStatusTabs";

const counts = {
  all: 23,
  pending: 1,
  paid: 8,
  shipped: 1,
  completed: 12,
  cancelled: 1,
};

describe("OrderStatusTabs", () => {
  it("links each tab to its status without a page and shows counts", () => {
    render(<OrderStatusTabs active="paid" counts={counts} />);
    expect(screen.getByRole("link", { name: /Tất cả/ })).toHaveAttribute(
      "href",
      "/account/orders",
    );
    expect(screen.getByRole("link", { name: /Đã giao/ })).toHaveAttribute(
      "href",
      "/account/orders?status=completed",
    );
    expect(
      screen.getByRole("link", { name: /Đã thanh toán/ }),
    ).toHaveTextContent("8");
  });

  it("marks the active tab", () => {
    render(<OrderStatusTabs active="shipped" counts={counts} />);
    expect(screen.getByRole("link", { name: /Đang giao/ })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: /Tất cả/ })).not.toHaveAttribute(
      "aria-current",
    );
  });

  it("builds hrefs with status and page", () => {
    expect(ordersHref("all")).toBe("/account/orders");
    expect(ordersHref("all", 3)).toBe("/account/orders?page=3");
    expect(ordersHref("paid", 2)).toBe("/account/orders?status=paid&page=2");
    expect(ordersHref("paid", 1)).toBe("/account/orders?status=paid");
  });
});
