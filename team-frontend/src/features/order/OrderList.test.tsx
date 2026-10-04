import { render, screen } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";

import { OrderList } from "./OrderList";
import { makeOrder } from "./orderFixtures";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("./actions", () => ({
  reorderAction: vi.fn(),
  cancelOrderAction: vi.fn(),
  createReturnRequestAction: vi.fn(),
  mockRefundAction: vi.fn(),
}));
vi.mock("@/features/review/ReviewModal", () => ({ ReviewModal: () => null }));

describe("OrderList", () => {
  it("renders a card per order with badge, items, total and actions", () => {
    render(
      <OrderList
        orders={[
          makeOrder("order-aaaa-1111"),
          makeOrder("order-bbbb-2222", OrderStatus.SHIPPED),
        ]}
        status="all"
        shopNames={new Map()}
      />,
    );
    expect(screen.getAllByTestId("order-card")).toHaveLength(2);
    expect(
      screen.getByText("Mã đơn: #order-aa", { exact: false }),
    ).toBeInTheDocument();
    expect(screen.getByText("Chờ xử lý")).toBeInTheDocument();
    expect(screen.getByText("Đang giao hàng")).toBeInTheDocument();
    expect(
      screen.getAllByRole("link", { name: "Xem chi tiết" })[0],
    ).toHaveAttribute("href", "/account/orders/order-aaaa-1111");
    expect(screen.getAllByRole("button", { name: "Mua lại" })).toHaveLength(2);
    // Cancel only on the pending order.
    expect(screen.getAllByRole("button", { name: "Hủy đơn" })).toHaveLength(1);
  });

  it("offers review only on completed orders", () => {
    render(
      <OrderList
        orders={[
          makeOrder("o1", OrderStatus.COMPLETED),
          makeOrder("o2", OrderStatus.PAID),
        ]}
        status="all"
        shopNames={new Map()}
      />,
    );
    expect(screen.getAllByRole("button", { name: "Đánh giá" })).toHaveLength(1);
  });

  it("shows the real shop display name", () => {
    render(
      <OrderList
        orders={[makeOrder("o1")]}
        status="all"
        shopNames={new Map([["seller-abcdef-123", "Cửa hàng Hoa Mai"]])}
      />,
    );
    expect(screen.getByText("Cửa hàng Hoa Mai")).toBeInTheDocument();
    expect(screen.queryByText(/Shop #/)).toBeNull();
  });

  it("falls back to Shop #<6 chars> for an empty name", () => {
    render(
      <OrderList
        orders={[makeOrder("o1")]}
        status="all"
        shopNames={new Map([["seller-abcdef-123", "  "]])}
      />,
    );
    expect(screen.getByText("Shop #seller")).toBeInTheDocument();
  });

  it("loads the first card's thumbnails eagerly and the rest lazily", () => {
    render(
      <OrderList
        orders={[
          makeOrder("o1"),
          makeOrder("o2"),
          makeOrder("o3"),
          makeOrder("o4"),
        ]}
        status="all"
        shopNames={new Map()}
      />,
    );
    const imgs = screen.getAllByRole("img", { name: "Áo thun" });
    expect(imgs).toHaveLength(4);
    expect(imgs[0]).toHaveAttribute("loading", "eager");
    for (const img of imgs.slice(1)) {
      expect(img).toHaveAttribute("loading", "lazy");
    }
  });

  it("keeps the thumbnail box when an item has no image", () => {
    const order = makeOrder("o1");
    order.items[0].imageUrl = "";
    render(<OrderList orders={[order]} status="all" shopNames={new Map()} />);
    expect(screen.getByRole("img", { name: "Không có ảnh" })).toHaveClass(
      "aspect-square",
    );
  });

  it("shows Empty with a shopping action for the all tab", () => {
    render(<OrderList orders={[]} status="all" shopNames={new Map()} />);
    expect(screen.getByText("Bạn chưa có đơn hàng nào")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Mua sắm ngay" })).toHaveAttribute(
      "href",
      "/",
    );
  });

  it("shows Empty with a way back for another tab", () => {
    render(<OrderList orders={[]} status="cancelled" shopNames={new Map()} />);
    expect(
      screen.getByRole("link", { name: "Xem tất cả đơn hàng" }),
    ).toHaveAttribute("href", "/account/orders");
  });
});
