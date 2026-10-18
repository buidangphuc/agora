import { render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";
import { OrderDetailView } from "./OrderDetailView";
import { SellerOrdersTable } from "./SellerOrdersTable";

vi.mock("@/features/order/actions", () => ({
  updateOrderStatusAction: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));

const PAST = "2026-10-10T03:00:00.000Z";
const FUTURE = "2026-10-25T03:00:00.000Z";

function order(id: string, over: Partial<ViewOrder>): ViewOrder {
  return {
    id,
    buyerId: "b1",
    sellerId: "s1",
    status: OrderStatus.PAID,
    statusText: "Đã thanh toán",
    totalAmount: 100000,
    itemsSubtotal: 100000,
    shippingFee: 0,
    paymentMethod: PaymentMethod.MOCK_MOMO,
    paymentMethodText: "MoMo",
    currency: "VND",
    discountAmount: 0,
    voucherCode: "",
    recipientName: "An",
    phone: "0900000000",
    addressFull: "HN",
    trackingNumber: "",
    items: [],
    createdAt: "01/10/2026",
    paidAt: "",
    paymentDueAt: "",
    shipBy: "",
    shippedAt: "",
    completedAt: "",
    returnWindowEndsAt: "",
    autoCompleteAt: "",
    cancelReason: 0,
    ...over,
  } as ViewOrder;
}

const overdue = order("overdue-111", { shipBy: PAST });
const onTime = order("ontime-2222", { shipBy: FUTURE });
const online = order("online-3333", {
  status: OrderStatus.PENDING,
  statusText: "Chờ xử lý",
  shipBy: "",
});

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-17T10:00:00.000Z"));
});
afterEach(() => vi.useRealTimers());

const rowOf = (id: string) =>
  screen
    .getByRole("link", { name: `#${id.slice(0, 8)}` })
    .closest("tr") as HTMLElement;

describe("[Order lifecycle: seller orders show the ship-by deadline] list", () => {
  it("ship-by shown with overdue tag", () => {
    render(<SellerOrdersTable orders={[overdue, onTime, online]} />);

    const late = within(rowOf(overdue.id));
    expect(late.getByText(/Hạn gửi:/)).toBeInTheDocument();
    expect(rowOf(overdue.id)).toHaveTextContent(/2026/);
    expect(late.getByText("Quá hạn")).toBeInTheDocument();

    const ok = within(rowOf(onTime.id));
    expect(ok.getByText(/Hạn gửi:/)).toBeInTheDocument();
    expect(rowOf(onTime.id)).toHaveTextContent(/2026/);
    expect(ok.queryByText("Quá hạn")).toBeNull();

    const none = within(rowOf(online.id));
    expect(rowOf(online.id).textContent).toMatch(/Hạn gửi:\s*-/);
    expect(none.queryByText("Quá hạn")).toBeNull();
  });

  it("does not tag a shipped order past ship-by as overdue", () => {
    const shipped = order("shipped-444", {
      status: OrderStatus.SHIPPED,
      statusText: "Đang giao hàng",
      shipBy: PAST,
    });
    render(<SellerOrdersTable orders={[shipped]} />);
    expect(within(rowOf(shipped.id)).queryByText("Quá hạn")).toBeNull();
  });
});

describe("[Order lifecycle: seller orders show the ship-by deadline] detail", () => {
  const detail = (o: ViewOrder) =>
    render(
      <OrderDetailView
        order={o}
        shipment={null}
        tab="items"
        tabHref={(t) => `?tab=${t}`}
      />,
    );

  it("ship-by shown with overdue tag on the detail", () => {
    detail(overdue);
    const slip = screen.getByTestId("packing-slip");
    expect(slip).toHaveTextContent(/Hạn gửi/);
    expect(slip).toHaveTextContent(/2026/);
    expect(within(slip).getByText("Quá hạn")).toBeInTheDocument();
  });

  it("shows the date and no tag for a future ship-by", () => {
    detail(onTime);
    const slip = screen.getByTestId("packing-slip");
    expect(slip).toHaveTextContent(/Hạn gửi/);
    expect(within(slip).queryByText("Quá hạn")).toBeNull();
  });

  it("shows - and no tag without ship-by", () => {
    detail(online);
    const slip = screen.getByTestId("packing-slip");
    expect(slip).toHaveTextContent(/Hạn gửi/);
    expect(within(slip).queryByText("Quá hạn")).toBeNull();
  });
});
