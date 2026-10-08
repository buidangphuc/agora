import { render, screen, waitFor, within } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { setupUser } from "@/test/user";

import { OrderDetailView, orderSteps } from "./OrderDetailView";
import { ReturnStateProvider } from "./ReturnState";
import { cancelOrderAction } from "./actions";
import { makeOrder } from "./orderFixtures";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("./actions", () => ({
  reorderAction: vi.fn(),
  cancelOrderAction: vi.fn(),
  createReturnRequestAction: vi.fn(),
}));
vi.mock("@/features/review/ReviewModal", () => ({ ReviewModal: () => null }));
const toast = { success: vi.fn(), error: vi.fn(), info: vi.fn() };
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));

function view(order = makeOrder("order-1234-abcd")) {
  return (
    <ReturnStateProvider>
      <OrderDetailView
        order={order}
        timeline={<div data-testid="timeline-slot">timeline</div>}
      />
    </ReturnStateProvider>
  );
}

beforeEach(() => vi.clearAllMocks());

describe("OrderDetailView", () => {
  it("shows header, progress, descriptions, items and tabs for a shipped order", () => {
    render(
      view(
        makeOrder("order-1234-abcd", OrderStatus.SHIPPED, {
          trackingNumber: "TRK9",
          voucherCode: "SALE10",
          discountAmount: 5000,
        }),
      ),
    );
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Order #order-12",
    );
    expect(
      screen.getByRole("navigation", { name: "Breadcrumb" }),
    ).toHaveTextContent("Đơn hàng của tôi");
    expect(screen.getByTestId("order-status")).toHaveTextContent(
      "Đang giao hàng",
    );

    const current = screen
      .getAllByText("Đang vận chuyển")
      .map((el) => el.closest("[aria-current='step']"))
      .filter(Boolean);
    expect(current.length).toBeGreaterThan(0);

    expect(screen.getByText("Người nhận")).toBeInTheDocument();
    expect(screen.getByText("SALE10")).toBeInTheDocument();
    expect(screen.getByText("TRK9")).toBeInTheDocument();
    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByText("Áo thun")).toBeInTheDocument();
    expect(screen.getByText("Giảm giá")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Hành trình" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByTestId("timeline-slot")).toBeInTheDocument();
  });

  it("hides voucher, tracking and discount rows when there is no data", () => {
    render(view());
    expect(screen.queryByText("Mã giảm giá")).toBeNull();
    expect(screen.queryByText("Mã vận đơn")).toBeNull();
    expect(screen.queryByText("Giảm giá")).toBeNull();
  });

  it("offers Mua lại and Hủy đơn but not a return on a pending order", () => {
    render(view());
    expect(screen.getByRole("button", { name: "Mua lại" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Hủy đơn" })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Yêu cầu trả hàng" }),
    ).toBeNull();
  });

  it("offers a return and review, but no cancel, on a completed order", () => {
    render(view(makeOrder("o1", OrderStatus.COMPLETED)));
    expect(
      screen.getAllByRole("button", { name: "Yêu cầu trả hàng" }).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getByRole("button", { name: "Đánh giá" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Hủy đơn" })).toBeNull();
  });

  it("replaces the Stepper by an Alert on a cancelled order", () => {
    render(view(makeOrder("o1", OrderStatus.CANCELLED)));
    expect(screen.getByText("Đơn hàng đã hủy")).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Progress" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Hủy đơn" })).toBeNull();
  });

  it("cancel calls cancelOrderAction once and the badge follows the revalidated order", async () => {
    vi.mocked(cancelOrderAction).mockResolvedValue({ ok: true });
    const { rerender } = render(view());
    const user = setupUser();
    await user.click(screen.getByRole("button", { name: "Hủy đơn" }));
    await user.click(screen.getByTestId("cancel-confirm"));

    await waitFor(() => expect(cancelOrderAction).toHaveBeenCalledTimes(1));
    expect(cancelOrderAction).toHaveBeenCalledWith(
      "order-1234-abcd",
      expect.any(String),
    );
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    // The page is re-rendered from revalidated server data.
    expect(screen.getByTestId("order-status")).toHaveTextContent("Chờ xử lý");
    rerender(view(makeOrder("order-1234-abcd", OrderStatus.CANCELLED)));
    expect(screen.getByTestId("order-status")).toHaveTextContent("Đã hủy");
  });

  it("returns tab shows Empty for an order without a return", () => {
    render(view(makeOrder("o1", OrderStatus.COMPLETED)));
    const panel = screen.getByTestId("return-section");
    expect(
      within(panel).getByText("Chưa có yêu cầu trả hàng"),
    ).toBeInTheDocument();
  });
});

describe("OrderDetailView markup", () => {
  it("has no invalid DOM nesting (a hydration error in the browser)", () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      view(makeOrder("o1", OrderStatus.COMPLETED, { discountAmount: 1000 })),
    );
    expect(error).not.toHaveBeenCalled();
    error.mockRestore();
  });
});

describe("orderSteps", () => {
  it("marks steps before the current one complete", () => {
    expect(orderSteps(OrderStatus.SHIPPED).map((s) => s.status)).toEqual([
      "complete",
      "complete",
      "complete",
      "current",
      "upcoming",
    ]);
  });

  it("marks every step complete when the order is completed", () => {
    expect(
      orderSteps(OrderStatus.COMPLETED).every((s) => s.status === "complete"),
    ).toBe(true);
  });
});
