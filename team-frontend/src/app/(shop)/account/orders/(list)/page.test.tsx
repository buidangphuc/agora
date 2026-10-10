import { fireEvent, render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { makeOrder } from "@/features/order/orderFixtures";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { listBuyerOrdersResult } from "@/lib/gateway/orders";
import { getPrincipal } from "@/lib/gateway/session";
import { batchGetShopNames } from "@/lib/gateway/shops";

import AccountOrdersPage from "./page";

vi.mock("@/lib/gateway/orders", () => ({ listBuyerOrdersResult: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/shops", async (orig) => ({
  ...(await orig<typeof import("@/lib/gateway/shops")>()),
  batchGetShopNames: vi.fn(),
}));
const refresh = vi.hoisted(() => vi.fn());
vi.mock("next/navigation", () => ({
  redirect: vi.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
  useRouter: () => ({ push: vi.fn(), refresh }),
}));
vi.mock("@/features/order/actions", () => ({
  reorderAction: vi.fn(),
  cancelOrderAction: vi.fn(),
  createReturnRequestAction: vi.fn(),
}));
vi.mock("@/features/review/ReviewModal", () => ({ ReviewModal: () => null }));

const orders = [
  ...Array.from({ length: 12 }, (_, i) =>
    makeOrder(`done-${i}`, OrderStatus.COMPLETED),
  ),
  ...Array.from({ length: 8 }, (_, i) =>
    makeOrder(`paid-${i}`, OrderStatus.PAID),
  ),
  makeOrder("canc-0", OrderStatus.CANCELLED),
  makeOrder("pend-0", OrderStatus.PENDING),
  makeOrder("ship-0", OrderStatus.SHIPPED),
];

async function renderPage(
  searchParams: { status?: string; page?: string } = {},
) {
  render(await AccountOrdersPage({ searchParams }));
}

beforeEach(() => {
  refresh.mockClear();
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({ userId: "b1" } as never);
  vi.mocked(batchGetShopNames).mockResolvedValue(new Map());
  vi.mocked(listBuyerOrdersResult).mockResolvedValue({ ok: true, orders });
});

describe("/account/orders page", () => {
  it("renders tabs with counts, 10 rows and a 3-page pagination", async () => {
    await renderPage();
    expect(
      screen.getByRole("heading", { name: "Đơn hàng của tôi" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Đã giao/ })).toHaveTextContent(
      "12",
    );
    expect(screen.getAllByTestId("order-card")).toHaveLength(10);
    expect(screen.getByRole("link", { name: "Trang 3" })).toHaveAttribute(
      "href",
      "/account/orders?page=3",
    );
    expect(listBuyerOrdersResult).toHaveBeenCalledTimes(1);
  });

  it("shows the last 3 orders on page 3", async () => {
    await renderPage({ page: "3" });
    expect(screen.getAllByTestId("order-card")).toHaveLength(3);
    expect(screen.getByRole("link", { name: "Trang 3" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("filters by status and keeps the status in pagination links", async () => {
    await renderPage({ status: "completed", page: "2" });
    expect(screen.getAllByTestId("order-card")).toHaveLength(2);
    expect(screen.getByRole("link", { name: "Trang 1" })).toHaveAttribute(
      "href",
      "/account/orders?status=completed",
    );
  });

  it("falls back to all for an invalid query without an error", async () => {
    await renderPage({ status: "bogus", page: "99" });
    expect(screen.getByRole("link", { name: /Tất cả/ })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getAllByTestId("order-card")).toHaveLength(3);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("shows Empty with zero counts for a buyer with no orders", async () => {
    vi.mocked(listBuyerOrdersResult).mockResolvedValue({
      ok: true,
      orders: [],
    });
    await renderPage();
    expect(screen.getByText("Bạn chưa có đơn hàng nào")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Tất cả/ })).toHaveTextContent("0");
  });

  it("shows an Alert with retry, not the empty state, when the load fails", async () => {
    vi.mocked(listBuyerOrdersResult).mockResolvedValue({ ok: false });
    await renderPage({ status: "paid" });
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Không tải được danh sách đơn hàng",
    );
    // A same-URL link would be served from the router cache; the retry must be
    // a button that re-fetches via router.refresh().
    expect(screen.queryByRole("link", { name: "Thử lại" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
    expect(refresh).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("Bạn chưa có đơn hàng nào")).toBeNull();
  });

  it("looks up shop names only for the visible orders", async () => {
    await renderPage({ page: "3" });
    const ids = vi.mocked(batchGetShopNames).mock.calls[0][0] as string[];
    expect([...ids]).toHaveLength(3);
  });

  it("redirects a guest to /login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null as never);
    await expect(AccountOrdersPage({ searchParams: {} })).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirect).toHaveBeenCalledWith("/login");
  });
});
