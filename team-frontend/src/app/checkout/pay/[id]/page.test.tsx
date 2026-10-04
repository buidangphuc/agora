import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import {
  PaymentMethod,
  PaymentStatus,
} from "@/generated/platform/payment/v1/payment_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";
import { getOrder } from "@/lib/gateway/orders";
import type { ViewPaymentTransaction } from "@/lib/gateway/payment";
import { getPayment } from "@/lib/gateway/payment";
import { getPrincipal } from "@/lib/gateway/session";

import PaymentLoading from "./loading";
import PaymentNotFound from "./not-found";
import MockPaymentPage from "./page";

class Signal extends Error {}
vi.mock("next/navigation", () => ({
  redirect: (url: string) => {
    throw new Signal(`redirect:${url}`);
  },
  notFound: () => {
    throw new Signal("notFound");
  },
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/orders", () => ({ getOrder: vi.fn() }));
vi.mock("@/lib/gateway/payment", () => ({ getPayment: vi.fn() }));
vi.mock("@/features/order/actions", () => ({
  processMockPaymentAction: vi.fn(),
}));

function order(status: OrderStatus): ViewOrder {
  return { id: "order-12345678", status } as ViewOrder;
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
    render(await MockPaymentPage({ params: { id } }));
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
    expect(await outcome()).toBe("redirect:/login");
  });

  it("calls notFound for an unknown order", async () => {
    vi.mocked(getOrder).mockResolvedValue(null);
    expect(await outcome("does-not-exist")).toBe("notFound");
  });

  it("calls notFound when the order has no transaction", async () => {
    vi.mocked(getPayment).mockResolvedValue(null);
    expect(await outcome()).toBe("notFound");
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

  it("explains a saga-cancelled order with an Alert and a link to the orders", async () => {
    vi.mocked(getOrder).mockResolvedValue(order(OrderStatus.CANCELLED));
    await outcome();
    expect(screen.getByText("Đơn hàng đã bị hủy")).toBeInTheDocument();
    expect(screen.getByText(/tồn kho đã được giải phóng/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Xem đơn hàng" })).toHaveAttribute(
      "href",
      "/account/orders",
    );
    expect(
      screen.queryByRole("button", { name: "Thanh toán thành công" }),
    ).toBeNull();
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
