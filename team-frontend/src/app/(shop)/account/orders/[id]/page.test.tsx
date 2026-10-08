import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { makeOrder } from "@/features/order/orderFixtures";
import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";
import {
  getOrderResult,
  getSagaState,
  getShipmentTracking,
  listOrderReturns,
} from "@/lib/gateway/orders";
import { getPrincipal } from "@/lib/gateway/session";

import OrderNotFound from "../not-found";
import BuyerOrderDetailPage from "./page";

vi.mock("@/lib/gateway/orders", () => ({
  getOrderResult: vi.fn(),
  getShipmentTracking: vi.fn(),
  getSagaState: vi.fn(),
  listOrderReturns: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("next/navigation", () => ({
  redirect: vi.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
  useRouter: () => ({ push: vi.fn() }),
}));
vi.mock("@/features/order/actions", () => ({
  reorderAction: vi.fn(),
  cancelOrderAction: vi.fn(),
  createReturnRequestAction: vi.fn(),
}));
vi.mock("@/features/review/ReviewModal", () => ({ ReviewModal: () => null }));
// The async timeline loader cannot render in jsdom; the page only needs its props.
vi.mock("@/features/order/OrderTimelineSection", () => ({
  OrderTimelineSection: ({ orderId }: { orderId: string }) => (
    <div data-testid="timeline-section">{orderId}</div>
  ),
  OrderTimelineSkeleton: () => <div data-testid="timeline-skeleton" />,
}));

async function renderPage(id = "o1", tab?: string) {
  render(await BuyerOrderDetailPage({ params: { id }, searchParams: { tab } }));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({ id: "b1" } as never);
  vi.mocked(listOrderReturns).mockResolvedValue([]);
});

describe("/account/orders/[id] page", () => {
  it("renders the order and hands the timeline its order id", async () => {
    vi.mocked(getOrderResult).mockResolvedValue({
      kind: "ok",
      order: makeOrder("o1"),
    });
    await renderPage();
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Order #o1",
    );
    expect(screen.getByTestId("timeline-section")).toHaveTextContent("o1");
  });

  it("renders the order's returns from server data, with no refund button", async () => {
    vi.mocked(getOrderResult).mockResolvedValue({
      kind: "ok",
      order: makeOrder("o1"),
    });
    vi.mocked(listOrderReturns).mockResolvedValue([
      {
        id: "r1",
        orderId: "o1",
        reason: "hỏng",
        refundAmount: 50000,
        status: ReturnStatus.REFUNDED,
        statusText: "Đã hoàn tiền",
      },
    ]);
    await renderPage("o1", "returns");
    expect(listOrderReturns).toHaveBeenCalledWith("o1");
    expect(screen.getByTestId("return-status")).toHaveTextContent(
      "Đã hoàn tiền",
    );
    expect(screen.queryByRole("button", { name: /Hoàn tiền/ })).toBeNull();
  });

  it("opens the returns tab from ?tab=returns", async () => {
    vi.mocked(getOrderResult).mockResolvedValue({
      kind: "ok",
      order: makeOrder("o1"),
    });
    await renderPage("o1", "returns");
    expect(
      screen.getByRole("tab", { name: "Trả hàng / Hoàn tiền" }),
    ).toHaveAttribute("aria-selected", "true");
  });

  it("renders a 403 Result with no order data and fetches nothing else", async () => {
    vi.mocked(getOrderResult).mockResolvedValue({ kind: "forbidden" });
    await renderPage();
    expect(
      screen.getByRole("heading", {
        name: "Bạn không có quyền xem đơn hàng này",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Về đơn hàng của tôi" }),
    ).toHaveAttribute("href", "/account/orders");
    expect(screen.queryByText("Áo thun")).toBeNull();
    expect(screen.queryByText("An")).toBeNull();
    expect(getShipmentTracking).not.toHaveBeenCalled();
    expect(getSagaState).not.toHaveBeenCalled();
  });

  it("renders a 404 Result for a missing order", async () => {
    vi.mocked(getOrderResult).mockResolvedValue({ kind: "not_found" });
    await renderPage("does-not-exist");
    expect(
      screen.getByRole("heading", { name: "Không tìm thấy đơn hàng" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Về đơn hàng của tôi" }),
    ).toHaveAttribute("href", "/account/orders");
  });

  it("renders an Alert with retry, not a 404, on a transport error", async () => {
    vi.mocked(getOrderResult).mockResolvedValue({ kind: "error" });
    await renderPage("o9");
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Thử lại" })).toHaveAttribute(
      "href",
      "/account/orders/o9",
    );
    expect(screen.queryByText("Không tìm thấy đơn hàng")).toBeNull();
  });

  it("redirects a guest to /login before loading anything", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null as never);
    await expect(renderPage()).rejects.toThrow("NEXT_REDIRECT");
    expect(redirect).toHaveBeenCalledWith("/login");
    expect(getOrderResult).not.toHaveBeenCalled();
  });

  it("not-found.tsx renders the 404 Result", () => {
    render(<OrderNotFound />);
    expect(
      screen.getByRole("heading", { name: "Không tìm thấy đơn hàng" }),
    ).toBeInTheDocument();
  });
});

describe("order route boundaries", () => {
  it("no page under the order routes is a client component", () => {
    const root = join(process.cwd(), "src/app/(shop)/account/orders");
    const pages: string[] = [];
    const walk = (dir: string) => {
      for (const name of readdirSync(dir)) {
        const full = join(dir, name);
        if (statSync(full).isDirectory()) walk(full);
        else if (name === "page.tsx") pages.push(full);
      }
    };
    walk(root);
    expect(pages).toHaveLength(2);
    for (const file of pages) {
      expect(readFileSync(file, "utf8")).not.toMatch(/["']use client["']/);
    }
  });
});
