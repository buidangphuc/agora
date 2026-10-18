import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  CancelReason,
  OrderStatus,
} from "@/generated/platform/order/v1/order_pb.js";
import {
  PaymentMethod,
  PaymentStatus,
} from "@/generated/platform/payment/v1/payment_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";
import { getOrder } from "@/lib/gateway/orders";
import type { ViewPaymentTransaction } from "@/lib/gateway/payment";
import { createPayment, getPayment } from "@/lib/gateway/payment";
import { getPrincipal } from "@/lib/gateway/session";

import PaymentNotFound from "../not-found";
import PaymentLoading from "./loading";
import MockPaymentPage from "./page";

class Signal extends Error {}
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }),
  redirect: (url: string) => {
    throw new Signal(`redirect:${url}`);
  },
  notFound: () => {
    throw new Signal("notFound");
  },
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/orders", () => ({ getOrder: vi.fn() }));
vi.mock("@/lib/gateway/payment", () => ({
  getPayment: vi.fn(),
  createPayment: vi.fn(),
}));
vi.mock("@/features/order/actions", () => ({
  processMockPaymentAction: vi.fn(),
}));

const FUTURE = "2999-01-01T00:00:00.000Z";
const PAST = "2000-01-01T00:00:00.000Z";

function order(status: OrderStatus, extra: Partial<ViewOrder> = {}): ViewOrder {
  return {
    id: "order-12345678",
    status,
    totalAmount: 330000,
    paymentMethod: PaymentMethod.MOCK_MOMO,
    paymentMethodText: "Ví điện tử MoMo (Demo)",
    paymentDueAt: FUTURE,
    cancelReason: CancelReason.UNSPECIFIED,
    ...extra,
  } as ViewOrder;
}

function noPaymentCalls() {
  expect(getPayment).not.toHaveBeenCalled();
  expect(createPayment).not.toHaveBeenCalled();
  expect(
    screen.queryByRole("button", { name: "Thanh toán thành công" }),
  ).toBeNull();
  expect(
    screen.queryByRole("button", { name: "Thanh toán thất bại" }),
  ).toBeNull();
}

function tx(status: PaymentStatus): ViewPaymentTransaction {
  return {
    id: "tx1",
    orderId: "order-12345678",
    amount: 330000,
    currency: "VND",
    method: PaymentMethod.MOCK_MOMO,
    methodText: "Ví điện tử MoMo (Demo)",
    status,
  } as ViewPaymentTransaction;
}

async function outcome(id = "order-12345678") {
  try {
    render(await MockPaymentPage({ params: Promise.resolve({ id }) }));
    return null;
  } catch (e) {
    if (e instanceof Signal) return e.message;
    throw e;
  }
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({ id: "u1", name: "U", scopes: [] });
  vi.mocked(getOrder).mockResolvedValue(order(OrderStatus.PENDING));
  vi.mocked(getPayment).mockResolvedValue(tx(PaymentStatus.PENDING));
});

describe("/checkout/pay/[id]", () => {
  it("redirects a signed-out visitor to /login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    expect(await outcome()).toBe(
      "redirect:/login?returnUrl=%2Fcheckout%2Fpay%2Forder-12345678",
    );
  });

  it("calls notFound for an unknown order", async () => {
    vi.mocked(getOrder).mockResolvedValue(null);
    expect(await outcome("does-not-exist")).toBe("notFound");
  });

  it("creates a transaction when none exists", async () => {
    vi.mocked(getPayment).mockResolvedValue(null);
    vi.mocked(createPayment).mockResolvedValue({
      transaction: tx(PaymentStatus.PENDING),
      paymentUrl: "",
    });
    expect(await outcome()).toBeNull();
    expect(createPayment).toHaveBeenCalledTimes(1);
    expect(createPayment).toHaveBeenCalledWith(
      "order-12345678",
      PaymentMethod.MOCK_MOMO,
    );
    expect(
      screen.getByRole("button", { name: "Thanh toán thành công" }),
    ).toBeEnabled();
  });

  it("failed transaction is reused and the order is kept", async () => {
    vi.mocked(getPayment).mockResolvedValue(tx(PaymentStatus.FAILED));
    await outcome();
    expect(createPayment).not.toHaveBeenCalled();
    expect(screen.getByText(/Đơn hàng vẫn được giữ đến/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Thử lại" })).toBeInTheDocument();
  });

  it("no false cancellation claim", async () => {
    vi.mocked(getPayment).mockResolvedValue(tx(PaymentStatus.FAILED));
    await outcome();
    const text = document.body.textContent ?? "";
    expect(text.toLowerCase()).not.toContain("đơn hàng đã bị hủy");
    expect(text.toLowerCase()).not.toContain("hoàn tác");
  });

  it("expired order shows the expiry message", async () => {
    vi.mocked(getOrder).mockResolvedValue(
      order(OrderStatus.CANCELLED, {
        cancelReason: CancelReason.PAYMENT_EXPIRED,
      }),
    );
    const first = render(
      await MockPaymentPage({
        params: Promise.resolve({ id: "order-12345678" }),
      }),
    );
    expect(
      screen.getByText("Đơn hàng đã hết hạn thanh toán"),
    ).toBeInTheDocument();
    noPaymentCalls();
    first.unmount();

    vi.mocked(getOrder).mockResolvedValue(
      order(OrderStatus.PENDING, { paymentDueAt: PAST }),
    );
    await outcome();
    expect(
      screen.getByText("Đơn hàng đã hết hạn thanh toán"),
    ).toBeInTheDocument();
    noPaymentCalls();
  });

  it("cod order offers no online payment", async () => {
    vi.mocked(getOrder).mockResolvedValue(
      order(OrderStatus.PENDING, {
        paymentMethod: PaymentMethod.COD,
        paymentMethodText: "Thanh toán khi nhận hàng (COD)",
      }),
    );
    await outcome();
    expect(
      screen.getByText("Đơn hàng thanh toán khi nhận hàng"),
    ).toBeInTheDocument();
    noPaymentCalls();
  });

  it("unreadable order makes no payment call", async () => {
    vi.mocked(getOrder).mockResolvedValue(null);
    expect(await outcome("someone-elses")).toBe("notFound");
    expect(getPayment).not.toHaveBeenCalled();
    expect(createPayment).not.toHaveBeenCalled();
  });

  it.each([
    ["PAID", OrderStatus.PAID],
    ["SHIPPED", OrderStatus.SHIPPED],
    ["COMPLETED", OrderStatus.COMPLETED],
  ])(
    "non-pending order renders its state without payment calls (%s)",
    async (_n, status) => {
      vi.mocked(getOrder).mockResolvedValue(order(status));
      expect(await outcome()).toBeNull();
      expect(
        screen.getByText("Đơn hàng đã được thanh toán"),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("link", { name: "Xem đơn hàng" }),
      ).toHaveAttribute("href", "/account/orders/order-12345678");
      expect(screen.getByText("330.000")).toBeInTheDocument();
      expect(screen.getByText("Ví điện tử MoMo (Demo)")).toBeInTheDocument();
      noPaymentCalls();
    },
  );

  it("non-pending order renders its state without payment calls (cancelled, changed mind)", async () => {
    vi.mocked(getOrder).mockResolvedValue(
      order(OrderStatus.CANCELLED, {
        cancelReason: CancelReason.CHANGED_MIND,
      }),
    );
    expect(await outcome()).toBeNull();
    expect(screen.getByText("Đơn hàng đã bị hủy")).toBeInTheDocument();
    expect(
      screen.getByText("Đơn hàng đã bị hủy và tồn kho đã được giải phóng."),
    ).toBeInTheDocument();
    expect(screen.getByText("330.000")).toBeInTheDocument();
    noPaymentCalls();
  });

  it("shows the summary and the simulator for a pending transaction", async () => {
    expect(await outcome()).toBeNull();
    expect(screen.getByText("#order-12")).toBeInTheDocument();
    expect(screen.getByText("Ví điện tử MoMo (Demo)")).toBeInTheDocument();
    expect(screen.getByText("330.000")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Thanh toán thành công" }),
    ).toBeEnabled();
    expect(
      screen.getByRole("button", { name: "Thanh toán thất bại" }),
    ).toBeEnabled();
  });

  it("shows the success Result for a PAID transaction", async () => {
    vi.mocked(getPayment).mockResolvedValue(tx(PaymentStatus.PAID));
    await outcome();
    expect(screen.getByRole("link", { name: "Xem đơn hàng" })).toHaveAttribute(
      "href",
      "/account/orders",
    );
    expect(
      screen.queryByRole("button", { name: "Thanh toán thành công" }),
    ).toBeNull();
  });

  it("shows the error Result with recovery actions for a FAILED transaction", async () => {
    vi.mocked(getPayment).mockResolvedValue(tx(PaymentStatus.FAILED));
    await outcome();
    expect(screen.getByRole("button", { name: "Thử lại" })).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Đổi phương thức" }),
    ).toBeInTheDocument();
  });
});

describe("payment route states", () => {
  it("not-found renders a 404 Result linking to /account/orders", () => {
    render(<PaymentNotFound />);
    expect(screen.getByText("404")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Xem đơn hàng" })).toHaveAttribute(
      "href",
      "/account/orders",
    );
  });

  it("loading reserves the summary and simulator cards", () => {
    render(<PaymentLoading />);
    const root = screen.getByTestId("payment-skeleton");
    expect(root).toHaveAttribute("aria-busy", "true");
    expect(
      root.querySelectorAll("[data-variant]").length,
    ).toBeGreaterThanOrEqual(4);
  });
});
