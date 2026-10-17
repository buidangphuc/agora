import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { getShipmentTracking } from "@/lib/gateway/orders";
import { getPrincipal } from "@/lib/gateway/session";
import { loadSellerOrder } from "./data";
import SellerOrderDetailPage from "./page";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn(), refresh: vi.fn() }),
  usePathname: () => "/seller/orders/o1",
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/orders", () => ({
  getShipmentTracking: vi.fn(),
  listOrderReturns: vi.fn(),
}));
vi.mock("@/lib/gateway/payment", () => ({ getPayment: vi.fn() }));
vi.mock("./data", async () => {
  const actual = await vi.importActual<typeof import("./data")>("./data");
  return { ...actual, loadSellerOrder: vi.fn() };
});
vi.mock("@/features/seller/actions", () => ({
  shipOrderAction: vi.fn(),
}));

function shippedOrder(over: Record<string, unknown> = {}) {
  return {
    id: "o1234567-aaaa",
    buyerId: "buyer-1",
    sellerId: "seller-1",
    status: OrderStatus.SHIPPED,
    statusText: "Đang giao",
    totalAmount: 250000,
    itemsSubtotal: 250000,
    shippingFee: 0,
    paymentMethod: 0,
    paymentMethodText: "COD",
    currency: "VND",
    discountAmount: 0,
    voucherCode: "",
    recipientName: "An",
    phone: "0900000000",
    addressFull: "1 Lê Lợi",
    trackingNumber: "TRK1",
    items: [],
    createdAt: "01/10/2026",
    paidAt: "",
    paymentDueAt: "",
    shipBy: "",
    shippedAt: "2026-10-02T00:00:00.000Z",
    completedAt: "",
    returnWindowEndsAt: "",
    autoCompleteAt: "2026-10-09T00:00:00.000Z",
    cancelReason: 0,
    ...over,
  };
}

async function renderPage(tab?: string) {
  const ui = await SellerOrderDetailPage({
    params: Promise.resolve({ id: "o1234567-aaaa" }),
    searchParams: Promise.resolve(tab ? { tab } : {}),
  });
  return render(ui);
}

describe("Order lifecycle: only the buyer view offers the receipt button", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(getPrincipal).mockReturnValue({
      id: "seller-1",
      scopes: [],
    } as never);
    vi.mocked(getShipmentTracking).mockResolvedValue(null as never);
  });

  it("seller view has no receipt button", async () => {
    vi.mocked(loadSellerOrder).mockResolvedValue(shippedOrder() as never);
    await renderPage();
    // The page really rendered the SHIPPED order for its seller...
    expect(screen.getAllByText(/Đơn hàng #o1234567/).length).toBeGreaterThan(0);
    // ...and it never offers the buyer-only receipt action.
    expect(screen.queryByText(/Đã nhận được hàng/)).toBeNull();
    expect(
      screen.queryByRole("button", { name: /Đã nhận được hàng/ }),
    ).toBeNull();
  });

  it("seller view has no receipt button for an admin reading it", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "admin-1",
      scopes: ["order.admin"],
    } as never);
    vi.mocked(loadSellerOrder).mockResolvedValue(shippedOrder() as never);
    await renderPage();
    expect(screen.getAllByText(/Đơn hàng #o1234567/).length).toBeGreaterThan(0);
    expect(screen.queryByText(/Đã nhận được hàng/)).toBeNull();
  });
});
