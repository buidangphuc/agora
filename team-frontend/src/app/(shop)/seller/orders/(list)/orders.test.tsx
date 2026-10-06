import { render, screen, within } from "@testing-library/react";
import { notFound } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import {
  getOrder,
  getShipmentTracking,
  listSellerOrders,
} from "@/lib/gateway/orders";
import { getPrincipal, hasScope } from "@/lib/gateway/session";
import SellerOrderDetailLoading from "../[id]/loading";
import SellerOrderDetailPage from "../[id]/page";
import SellerOrdersLoading from "./loading";
import SellerOrdersPage from "./page";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn() }),
  usePathname: () => "/seller/orders",
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  hasScope: vi.fn(),
}));
vi.mock("@/lib/gateway/orders", () => ({
  listSellerOrders: vi.fn(),
  getOrder: vi.fn(),
  getShipmentTracking: vi.fn(),
}));
vi.mock("@/features/order/actions", () => ({
  updateOrderStatusAction: vi.fn(),
}));

const STATUS_TEXT: Partial<Record<OrderStatus, string>> = {
  [OrderStatus.PENDING]: "Chờ xử lý",
  [OrderStatus.PAID]: "Đã thanh toán",
  [OrderStatus.SHIPPED]: "Đang giao hàng",
  [OrderStatus.COMPLETED]: "Đã hoàn thành",
};

function order(
  id: string,
  status: OrderStatus,
  extra: Record<string, unknown> = {},
) {
  return {
    id,
    sellerId: "me",
    status,
    statusText: STATUS_TEXT[status] ?? "?",
    recipientName: `Khách ${id}`,
    phone: "0900000000",
    addressFull: "18 Tràng Tiền, Hoàn Kiếm, Hà Nội",
    totalAmount: 250000,
    itemsSubtotal: 220000,
    shippingFee: 30000,
    discountAmount: 0,
    voucherCode: "",
    paymentMethodText: "COD",
    trackingNumber: "",
    createdAt: "01/10/2026",
    items: [
      {
        id: `it-${id}`,
        title: `Sản phẩm của ${id}`,
        variantName: "Đỏ",
        quantity: 2,
        unitPrice: 110000,
        imageUrl: "",
      },
    ],
    ...extra,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "me",
    name: "Me",
    scopes: ["listing.write"],
  });
  vi.mocked(hasScope).mockReturnValue(true);
  vi.mocked(getShipmentTracking).mockResolvedValue(null);
});

async function renderList(searchParams: Record<string, string> = {}) {
  render(await SellerOrdersPage({ searchParams }));
}

describe("/seller/orders", () => {
  beforeEach(() => {
    vi.mocked(listSellerOrders).mockResolvedValue([
      order("o1", OrderStatus.PENDING),
      order("o2", OrderStatus.SHIPPED),
      order("o3", OrderStatus.SHIPPED),
      order("o4", OrderStatus.COMPLETED),
    ] as never);
  });

  it("status tabs are links carrying counts; the active one comes from the URL", async () => {
    await renderList({ status: "shipped" });
    const tabs = screen.getByRole("navigation", { name: "Tabs" });
    const active = within(tabs)
      .getAllByRole("link")
      .filter((a) => a.getAttribute("aria-current") === "page");
    expect(active).toHaveLength(1);
    expect(active[0]).toHaveTextContent("Đang giao");
    expect(active[0]).toHaveTextContent("2");
    expect(within(tabs).getByRole("link", { name: /Tất cả/ })).toHaveAttribute(
      "href",
      "/seller/orders",
    );
    expect(
      within(tabs).getByRole("link", { name: /Hoàn thành/ }),
    ).toHaveAttribute("href", "/seller/orders?status=completed");
    // Only shipped orders are listed after "reload".
    expect(screen.getByRole("link", { name: "#o2" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "#o1" })).toBeNull();
    expect(screen.queryByRole("link", { name: "#o4" })).toBeNull();
  });

  it("offers Chi tiết for every row and ship only for shippable orders", async () => {
    await renderList();
    expect(screen.getAllByRole("link", { name: "Chi tiết" })).toHaveLength(4);
    expect(
      screen.getAllByRole("button", { name: "Xác nhận gửi" }),
    ).toHaveLength(1);
    expect(screen.getAllByRole("link", { name: "#o1" })[0]).toHaveAttribute(
      "href",
      "/seller/orders/o1",
    );
  });

  it("slices ?page= server-side and links neighbours", async () => {
    vi.mocked(listSellerOrders).mockResolvedValue(
      Array.from({ length: 45 }, (_, i) =>
        order(`ord${i + 1}`, OrderStatus.PENDING),
      ) as never,
    );
    await renderList({ page: "2" });
    expect(screen.getByRole("link", { name: "#ord21" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "#ord40" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "#ord41" })).toBeNull();
    const nav = screen.getByRole("navigation", { name: "Phân trang" });
    expect(within(nav).getByRole("link", { name: "Trang 2" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(
      within(nav).getByRole("link", { name: "Trang sau" }),
    ).toHaveAttribute("href", "/seller/orders?page=3");
  });

  it("falls back for invalid params", async () => {
    await renderList({ page: "abc", status: "bogus" });
    expect(screen.getAllByRole("link", { name: "Chi tiết" })).toHaveLength(4);
  });

  it("shows Empty with a clear-filter link when a search matches nothing", async () => {
    await renderList({ q: "khong-co" });
    expect(screen.getByText("Không có kết quả")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Xoá bộ lọc" })).toHaveAttribute(
      "href",
      "/seller/orders",
    );
  });

  it("shows an Alert with retry when the read fails, and no tab counts", async () => {
    vi.mocked(listSellerOrders).mockRejectedValue(new Error("down"));
    await renderList({ status: "pending" });
    expect(
      screen.getByText("Không tải được danh sách đơn hàng"),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Thử lại" })).toHaveAttribute(
      "href",
      "/seller/orders?status=pending",
    );
  });

  it("has no brand-filled primary button", async () => {
    await renderList();
    expect(document.querySelectorAll(".bg-action-primary").length).toBe(0);
  });
});

describe("/seller/orders/[id]", () => {
  async function renderDetail(id = "o1", searchParams: { tab?: string } = {}) {
    const ui = await SellerOrderDetailPage({ params: { id }, searchParams });
    if (ui) render(ui);
  }

  it("shows the real order: recipient, items and totals", async () => {
    vi.mocked(getOrder).mockResolvedValue(
      order("o1", OrderStatus.PAID) as never,
    );
    await renderDetail();
    const slip = screen.getByTestId("packing-slip");
    expect(within(slip).getByText("Khách o1")).toBeInTheDocument();
    expect(within(slip).getByText("0900000000")).toBeInTheDocument();
    expect(
      within(slip).getByText("18 Tràng Tiền, Hoàn Kiếm, Hà Nội"),
    ).toBeInTheDocument();
    expect(within(slip).getByText("Sản phẩm của o1")).toBeInTheDocument();
    // Nothing from the old hard-coded sample order.
    expect(screen.queryByText(/iPhone 15 Pro Max/)).toBeNull();
    expect(screen.queryByText(/Nguyễn Văn An/)).toBeNull();
    expect(screen.queryByText(/SPX-VN-88492019/)).toBeNull();
  });

  it("marks the current Stepper step", async () => {
    vi.mocked(getOrder).mockResolvedValue(
      order("o1", OrderStatus.PAID) as never,
    );
    await renderDetail();
    const steps = screen.getByRole("navigation", { name: "Progress" });
    const current = within(steps)
      .getAllByRole("listitem")
      .filter((li) => li.getAttribute("aria-current") === "step");
    expect(current).toHaveLength(1);
    expect(current[0]).toHaveTextContent("Đã thanh toán");
  });

  it("offers Bàn giao vận chuyển only while shippable", async () => {
    vi.mocked(getOrder).mockResolvedValue(
      order("o1", OrderStatus.PAID) as never,
    );
    await renderDetail();
    expect(
      screen.getByRole("button", { name: "Bàn giao vận chuyển" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "In phiếu" }),
    ).toBeInTheDocument();

    document.body.innerHTML = "";
    vi.mocked(getOrder).mockResolvedValue(
      order("o1", OrderStatus.SHIPPED) as never,
    );
    await renderDetail();
    expect(
      screen.queryByRole("button", { name: "Bàn giao vận chuyển" }),
    ).toBeNull();
  });

  it("the shipment tab shows a Timeline of checkpoints", async () => {
    vi.mocked(getOrder).mockResolvedValue(
      order("o1", OrderStatus.SHIPPED) as never,
    );
    vi.mocked(getShipmentTracking).mockResolvedValue({
      id: "s1",
      carrier: "SPX",
      trackingCode: "TRK9",
      statusText: "Đang vận chuyển",
      checkpoints: [
        { timestamp: "02/10", location: "Hà Nội", description: "Đã lấy hàng" },
      ],
    } as never);
    await renderDetail("o1", { tab: "shipment" });
    expect(screen.getByText("Đã lấy hàng")).toBeInTheDocument();
    expect(screen.getByText("TRK9")).toBeInTheDocument();
  });

  it("falls back to the seller's order list when getOrder is refused", async () => {
    vi.mocked(getOrder).mockResolvedValue(null);
    vi.mocked(listSellerOrders).mockResolvedValue([
      order("o1", OrderStatus.PENDING),
    ] as never);
    await renderDetail();
    expect(screen.getByText("Khách o1")).toBeInTheDocument();
  });

  it("an unknown order is a not-found state", async () => {
    vi.mocked(getOrder).mockResolvedValue(null);
    vi.mocked(listSellerOrders).mockResolvedValue([]);
    await renderDetail("does-not-exist");
    expect(notFound).toHaveBeenCalled();
  });

  it("an order of another seller is a not-found state", async () => {
    vi.mocked(getOrder).mockResolvedValue(
      order("o9", OrderStatus.PENDING, { sellerId: "someone-else" }) as never,
    );
    await renderDetail("o9");
    expect(notFound).toHaveBeenCalled();
  });

  it("includes a print stylesheet that hides the site chrome", async () => {
    vi.mocked(getOrder).mockResolvedValue(
      order("o1", OrderStatus.PAID) as never,
    );
    await renderDetail();
    const css = document.querySelector("style")?.textContent ?? "";
    expect(css).toContain("@media print");
    expect(css).toContain("header");
  });
});

describe("loading skeletons", () => {
  it("render busy skeletons for both order routes", () => {
    render(<SellerOrdersLoading />);
    expect(screen.getByTestId("seller-skeleton")).toHaveAttribute(
      "aria-busy",
      "true",
    );
    document.body.innerHTML = "";
    render(<SellerOrderDetailLoading />);
    expect(screen.getByTestId("seller-skeleton")).toHaveAttribute(
      "aria-busy",
      "true",
    );
  });
});
