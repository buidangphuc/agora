import { render, screen, waitFor, within } from "@testing-library/react";
import React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  CancelReason,
  OrderStatus,
  ReturnStatus,
} from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
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
  confirmReceiptAction: vi.fn(),
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

const NOW = new Date("2026-10-17T10:00:00.000Z");
const iso = (ms: number) => new Date(ms).toISOString();
const HOUR = 3_600_000;
const DAY = 24 * HOUR;

describe("OrderDetailView lifecycle", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(NOW);
  });
  afterEach(() => vi.useRealTimers());

  it("Order lifecycle: cancel is offered only for pending and paid orders - cancel only for pending and paid", () => {
    const expected: [OrderStatus, boolean][] = [
      [OrderStatus.PENDING, true],
      [OrderStatus.PAID, true],
      [OrderStatus.SHIPPED, false],
      [OrderStatus.COMPLETED, false],
      [OrderStatus.CANCELLED, false],
    ];
    for (const [status, offered] of expected) {
      const { unmount } = render(view(makeOrder("o1", status)));
      expect(
        screen.queryAllByRole("button", { name: "Hủy đơn" }).length > 0,
        `status ${status}`,
      ).toBe(offered);
      unmount();
    }
  });

  it("Order lifecycle: a shipped order offers receipt and return instead of cancel - shipped offers receipt and return", () => {
    render(view(makeOrder("o1", OrderStatus.SHIPPED)));
    expect(screen.queryByRole("button", { name: "Hủy đơn" })).toBeNull();
    expect(
      screen.getAllByRole("button", { name: "Yêu cầu trả hàng" }).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByRole("button", { name: "Đã nhận được hàng" }).length,
    ).toBeGreaterThan(0);
  });

  it("Order lifecycle: an unpaid online order shows its deadline and the retry link - unpaid online order shows deadline and retry link", () => {
    render(
      view(
        makeOrder("order-1234-abcd", OrderStatus.PENDING, {
          paymentMethod: PaymentMethod.MOCK_MOMO,
          paymentDueAt: iso(NOW.getTime() + HOUR),
        }),
      ),
    );
    expect(screen.getByText(/Hạn thanh toán:.*2026/)).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Thanh toán lại" }),
    ).toHaveAttribute("href", "/checkout/pay/order-1234-abcd");
    expect(screen.queryByText(/Thanh toán ngay/)).toBeNull();
  });

  it("Order lifecycle: a COD order shows no deadline and no pay action - cod order has no deadline or pay action", () => {
    render(view(makeOrder("o1", OrderStatus.PENDING)));
    expect(screen.queryByText(/Hạn thanh toán/)).toBeNull();
    expect(screen.queryByText(/Thanh toán lại/)).toBeNull();
    expect(screen.queryByText(/Đã hết hạn thanh toán/)).toBeNull();
  });

  it("Order lifecycle: the pay action disappears at the deadline instant - detail pay link becomes expiry text at the deadline", () => {
    const due = NOW.getTime() + HOUR;
    const order = makeOrder("o1", OrderStatus.PENDING, {
      paymentMethod: PaymentMethod.MOCK_MOMO,
      paymentDueAt: iso(due),
    });
    vi.setSystemTime(due - 1);
    const first = render(view(order));
    expect(
      screen.getByRole("link", { name: "Thanh toán lại" }),
    ).toBeInTheDocument();
    first.unmount();

    vi.setSystemTime(due);
    render(view(order));
    expect(screen.queryByRole("link", { name: "Thanh toán lại" })).toBeNull();
    expect(screen.getByText(/Đã hết hạn thanh toán/)).toBeInTheDocument();
  });

  it("Order lifecycle: an order with no deadline keeps its pay action - legacy order without deadline shows no deadline text", () => {
    render(
      view(
        makeOrder("o1", OrderStatus.PENDING, {
          paymentMethod: PaymentMethod.MOCK_MOMO,
          paymentDueAt: "",
        }),
      ),
    );
    expect(screen.queryByText(/Hạn thanh toán/)).toBeNull();
    expect(
      screen.getByRole("link", { name: "Thanh toán lại" }),
    ).toBeInTheDocument();
  });

  it("Order lifecycle: an expired order detail names the expiry - expired cancel names the expiry", () => {
    const expiredText =
      "Đơn hàng đã hết hạn thanh toán và được hệ thống hủy tự động.";
    const first = render(
      view(
        makeOrder("o1", OrderStatus.CANCELLED, {
          cancelReason: CancelReason.PAYMENT_EXPIRED,
        }),
      ),
    );
    expect(screen.getByText(expiredText)).toBeInTheDocument();
    first.unmount();

    render(
      view(
        makeOrder("o1", OrderStatus.CANCELLED, {
          cancelReason: CancelReason.CHANGED_MIND,
        }),
      ),
    );
    expect(screen.queryByText(expiredText)).toBeNull();
    expect(
      screen.getByText("Tồn kho sản phẩm đã được hoàn lại cho người bán."),
    ).toBeInTheDocument();
  });

  it("Order lifecycle: a shipped order states its automatic completion time - shipped shows the auto-complete time when set", () => {
    const first = render(
      view(
        makeOrder("o1", OrderStatus.SHIPPED, {
          autoCompleteAt: iso(NOW.getTime() + 7 * DAY),
        }),
      ),
    );
    expect(
      screen.getByText(/Tự động hoàn thành vào.*2026/),
    ).toBeInTheDocument();
    first.unmount();

    render(view(makeOrder("o1", OrderStatus.SHIPPED, { autoCompleteAt: "" })));
    expect(screen.queryByText(/Tự động hoàn thành vào/)).toBeNull();
  });

  it("Order lifecycle: a return is offered for shipped orders and inside the window - return action and window text inside the window", () => {
    const first = render(view(makeOrder("o1", OrderStatus.SHIPPED)));
    expect(
      screen.getAllByRole("button", { name: "Yêu cầu trả hàng" }).length,
    ).toBeGreaterThan(0);
    first.unmount();

    render(
      view(
        makeOrder("o1", OrderStatus.COMPLETED, {
          returnWindowEndsAt: iso(NOW.getTime() + DAY),
        }),
      ),
    );
    expect(
      screen.getAllByRole("button", { name: "Yêu cầu trả hàng" }).length,
    ).toBeGreaterThan(0);
    expect(screen.getByText(/Có thể trả hàng đến.*2026/)).toBeInTheDocument();
  });

  it("Order lifecycle: the return window closes at its end instant - window closed text replaces the return action", () => {
    const end = NOW.getTime() + DAY;
    const order = makeOrder("o1", OrderStatus.COMPLETED, {
      returnWindowEndsAt: iso(end),
    });
    vi.setSystemTime(end - 1);
    const first = render(view(order));
    expect(
      screen.getAllByRole("button", { name: "Yêu cầu trả hàng" }).length,
    ).toBeGreaterThan(0);
    first.unmount();

    vi.setSystemTime(end);
    render(view(order));
    expect(screen.getByText(/Đã hết thời hạn trả hàng/)).toBeInTheDocument();
    expect(
      screen.queryAllByRole("button", { name: "Yêu cầu trả hàng" }),
    ).toHaveLength(0);
  });

  it("Order lifecycle: a completed order without a window leaves the decision to the server - completed order without window shows no return deadline", () => {
    render(
      view(makeOrder("o1", OrderStatus.COMPLETED, { returnWindowEndsAt: "" })),
    );
    expect(screen.queryByText(/Có thể trả hàng đến/)).toBeNull();
    expect(
      screen.getAllByRole("button", { name: "Yêu cầu trả hàng" }).length,
    ).toBeGreaterThan(0);
  });

  it.each([
    ["PENDING", ReturnStatus.PENDING],
    ["APPROVED", ReturnStatus.APPROVED],
  ])(
    "Order lifecycle: a shipped order with an open return still offers receipt - receipt offered while a return is open (%s)",
    async (_n, status) => {
      render(
        <ReturnStateProvider
          initialReturns={[
            {
              id: "r1",
              orderId: "o1",
              reason: "lỗi",
              refundAmount: 1000,
              status,
              statusText: "x",
            },
          ]}
        >
          <OrderDetailView
            order={makeOrder("o1", OrderStatus.SHIPPED)}
            timeline={<div>timeline</div>}
          />
        </ReturnStateProvider>,
      );
      const buttons = screen.getAllByRole("button", {
        name: "Đã nhận được hàng",
      });
      expect(buttons).toHaveLength(1);
      const user = setupUser();
      await user.click(buttons[0]);
      expect(
        screen.getByText("Xác nhận bạn đã nhận được hàng?", { exact: false }),
      ).toBeInTheDocument();
      expect(screen.getByTestId("receipt-confirm")).toBeInTheDocument();
    },
  );

  it("Order lifecycle: a shipped order offers receipt and return instead of cancel - receipt is absent on non-shipped orders", () => {
    for (const st of [
      OrderStatus.PENDING,
      OrderStatus.PAID,
      OrderStatus.COMPLETED,
    ]) {
      const { unmount } = render(view(makeOrder("o1", st)));
      expect(
        screen.queryByRole("button", { name: "Đã nhận được hàng" }),
      ).toBeNull();
      unmount();
    }
  });
});
