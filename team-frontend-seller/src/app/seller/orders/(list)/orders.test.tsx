import { Code, ConnectError } from "@connectrpc/connect";
import { render, screen, within } from "@testing-library/react";
import { notFound } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import {
  getOrder,
  getShipmentTracking,
  listSellerOrders,
  listSellerOrdersPage,
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
  listSellerOrdersPage: vi.fn(),
  getOrder: vi.fn(),
  getShipmentTracking: vi.fn(),
}));
vi.mock("@/features/order/actions", () => ({
  updateOrderStatusAction: vi.fn(),
  bulkShipOrdersAction: vi.fn(),
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
  render(
    await SellerOrdersPage({ searchParams: Promise.resolve(searchParams) }),
  );
}

const ZERO_COUNTS = {
  [OrderStatus.PENDING]: 0,
  [OrderStatus.PAID]: 0,
  [OrderStatus.SHIPPED]: 0,
  [OrderStatus.COMPLETED]: 0,
  [OrderStatus.CANCELLED]: 0,
};

function pageOf(
  orders: ReturnType<typeof order>[],
  extra: Record<string, unknown> = {},
) {
  return {
    orders,
    nextCursor: "",
    total: orders.length,
    statusCounts: {
      ...ZERO_COUNTS,
      [OrderStatus.PENDING]: 1,
      [OrderStatus.SHIPPED]: 2,
      [OrderStatus.COMPLETED]: 1,
    },
    ...extra,
  };
}

function lastRequest() {
  return vi.mocked(listSellerOrdersPage).mock.calls[0][0] as {
    statuses?: OrderStatus[];
    query?: string;
    cursor?: string;
    pageSize?: number;
  };
}

describe("/seller/orders", () => {
  beforeEach(() => {
    vi.mocked(listSellerOrdersPage).mockResolvedValue(
      pageOf([
        order("o1", OrderStatus.PENDING),
        order("o2", OrderStatus.SHIPPED),
        order("o3", OrderStatus.SHIPPED),
        order("o4", OrderStatus.COMPLETED),
      ]) as never,
    );
  });

  it("First page of a large shop: one listSellerOrdersPage call, no full-list call, ALL badge from status_counts", async () => {
    vi.mocked(listSellerOrdersPage).mockResolvedValue(
      pageOf(
        Array.from({ length: 20 }, (_, i) =>
          order(`ord${i + 1}`, OrderStatus.PENDING),
        ),
        {
          nextCursor: "c20",
          total: 5000,
          statusCounts: {
            ...ZERO_COUNTS,
            [OrderStatus.PENDING]: 1000,
            [OrderStatus.PAID]: 1000,
            [OrderStatus.SHIPPED]: 1000,
            [OrderStatus.COMPLETED]: 1000,
            [OrderStatus.CANCELLED]: 1000,
          },
        },
      ) as never,
    );
    await renderList();
    expect(listSellerOrdersPage).toHaveBeenCalledTimes(1);
    expect(listSellerOrders).not.toHaveBeenCalled();
    expect(screen.getAllByRole("link", { name: "Chi tiết" })).toHaveLength(20);
    const tabs = screen.getByRole("navigation", { name: "Tabs" });
    expect(
      within(tabs).getByRole("link", { name: /Tất cả/ }),
    ).toHaveTextContent("5000");
    expect(lastRequest().pageSize).toBe(20);
    expect(lastRequest().cursor ?? "").toBe("");
    expect(lastRequest().statuses ?? []).toEqual([]);
  });

  it("Status tab filters on the server: the tab maps to statuses and the badge equals the SHIPPED count", async () => {
    vi.mocked(listSellerOrdersPage).mockResolvedValue(
      pageOf([
        order("o2", OrderStatus.SHIPPED),
        order("o3", OrderStatus.SHIPPED),
      ]) as never,
    );
    await renderList({ status: "shipped" });
    expect(lastRequest().statuses).toEqual([OrderStatus.SHIPPED]);
    const tabs = screen.getByRole("navigation", { name: "Tabs" });
    const active = within(tabs)
      .getAllByRole("link")
      .filter((a) => a.getAttribute("aria-current") === "page");
    expect(active).toHaveLength(1);
    expect(active[0]).toHaveTextContent("Đang giao");
    expect(active[0]).toHaveTextContent("2");
    expect(screen.getAllByText("Đang giao hàng")).toHaveLength(2);
    expect(screen.queryByRole("link", { name: "#o1" })).toBeNull();
  });

  it("Status tab filters on the server: Chờ xử lý sends PENDING and PAID", async () => {
    await renderList({ status: "pending" });
    expect(lastRequest().statuses).toEqual([
      OrderStatus.PENDING,
      OrderStatus.PAID,
    ]);
  });

  it("Badges stay stable while searching: query is sent and the ALL badge is the sum of status_counts", async () => {
    vi.mocked(listSellerOrdersPage).mockResolvedValue(
      pageOf([order("ab12cd34", OrderStatus.SHIPPED)], {
        statusCounts: {
          ...ZERO_COUNTS,
          [OrderStatus.PENDING]: 4,
          [OrderStatus.PAID]: 1,
          [OrderStatus.SHIPPED]: 1,
          [OrderStatus.COMPLETED]: 2,
          [OrderStatus.CANCELLED]: 3,
        },
      }) as never,
    );
    await renderList({ status: "shipped", q: "ab12cd34" });
    expect(lastRequest().query).toBe("ab12cd34");
    expect(
      screen.getByRole("link", { name: `#${"ab12cd34"}` }),
    ).toBeInTheDocument();
    const tabs = screen.getByRole("navigation", { name: "Tabs" });
    expect(
      within(tabs).getByRole("link", { name: /Tất cả/ }),
    ).toHaveTextContent("11");
    expect(
      within(tabs).getByRole("link", { name: /Chờ xử lý/ }),
    ).toHaveTextContent("5");
  });

  it("Next page uses the returned cursor: Trang sau links to next_cursor with page=2", async () => {
    vi.mocked(listSellerOrdersPage).mockResolvedValue(
      pageOf(
        Array.from({ length: 20 }, (_, i) =>
          order(`ord${i + 1}`, OrderStatus.PENDING),
        ),
        { nextCursor: "c20", total: 45 },
      ) as never,
    );
    await renderList();
    const nav = screen.getByRole("navigation", { name: "Phân trang" });
    expect(
      within(nav).getByRole("link", { name: "Trang sau" }),
    ).toHaveAttribute("href", "/seller/orders?cursor=c20&page=2");
  });

  it("Next page uses the returned cursor: page 2 requests the URL cursor and offers Trang đầu", async () => {
    vi.mocked(listSellerOrdersPage).mockResolvedValue(
      pageOf(
        Array.from({ length: 20 }, (_, i) =>
          order(`ord${i + 21}`, OrderStatus.PENDING),
        ),
        { nextCursor: "c40", total: 45 },
      ) as never,
    );
    await renderList({ cursor: "c20", page: "2" });
    expect(lastRequest().cursor).toBe("c20");
    expect(screen.getByRole("link", { name: "#ord21" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "#ord1" })).toBeNull();
    const nav = screen.getByRole("navigation", { name: "Phân trang" });
    expect(
      within(nav).getByRole("link", { name: "Trang đầu" }),
    ).toHaveAttribute("href", "/seller/orders");
    expect(within(nav).getByText("Trang 2")).toBeInTheDocument();
  });

  it("Last page has no next link: empty next_cursor hides Trang sau and shows 1 to 20 rows", async () => {
    vi.mocked(listSellerOrdersPage).mockResolvedValue(
      pageOf(
        Array.from({ length: 5 }, (_, i) =>
          order(`ord${i + 41}`, OrderStatus.PENDING),
        ),
        { nextCursor: "", total: 45 },
      ) as never,
    );
    await renderList({ cursor: "c40", page: "3" });
    expect(screen.queryByRole("link", { name: "Trang sau" })).toBeNull();
    expect(screen.getAllByRole("link", { name: "Chi tiết" })).toHaveLength(5);
  });

  it("Query is trimmed to 100 characters: the request carries 100 characters and the page renders", async () => {
    await renderList({ q: "z".repeat(150) });
    expect(lastRequest().query).toHaveLength(100);
    expect(screen.queryByText("Không tải được danh sách đơn hàng")).toBeNull();
    expect(screen.getAllByRole("link", { name: "Chi tiết" })).toHaveLength(4);
  });

  it("Orders page unknown cursor shows the retry state: Alert with a cursor-free Thử lại link", async () => {
    vi.mocked(listSellerOrdersPage).mockRejectedValue(
      new ConnectError("bad cursor", Code.InvalidArgument),
    );
    await renderList({ cursor: "not-a-cursor", page: "2" });
    expect(
      screen.getByText("Không tải được danh sách đơn hàng"),
    ).toBeInTheDocument();
    const retry = screen.getByRole("link", { name: "Thử lại" });
    expect(retry).toHaveAttribute("href", "/seller/orders");
    expect(retry.getAttribute("href")).not.toContain("cursor");
    expect(screen.queryByRole("link", { name: "Trang sau" })).toBeNull();
  });

  it("Orders page unknown cursor shows the retry state: retry keeps the tab but drops cursor and page", async () => {
    vi.mocked(listSellerOrdersPage).mockRejectedValue(new Error("down"));
    await renderList({ status: "pending", cursor: "bad", page: "4" });
    expect(screen.getByRole("link", { name: "Thử lại" })).toHaveAttribute(
      "href",
      "/seller/orders?status=pending",
    );
  });

  it("Changing tab or search resets the position: tab links carry neither cursor nor page", async () => {
    await renderList({ cursor: "c40", page: "3", q: "abc" });
    const tabs = screen.getByRole("navigation", { name: "Tabs" });
    const hrefs = within(tabs)
      .getAllByRole("link")
      .map((a) => a.getAttribute("href") ?? "");
    expect(hrefs).toHaveLength(4);
    for (const h of hrefs) {
      expect(h).not.toContain("cursor");
      expect(h).not.toContain("page");
    }
    expect(
      within(tabs).getByRole("link", { name: /Hoàn thành/ }),
    ).toHaveAttribute("href", "/seller/orders?q=abc&status=completed");
  });

  it("No match shows the empty result: Không có kết quả with Xoá bộ lọc to the bare list", async () => {
    vi.mocked(listSellerOrdersPage).mockResolvedValue(
      pageOf([], { total: 0, statusCounts: ZERO_COUNTS }) as never,
    );
    await renderList({ q: "khong-co" });
    expect(screen.getByText("Không có kết quả")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Xoá bộ lọc" })).toHaveAttribute(
      "href",
      "/seller/orders",
    );
  });

  it("First page of a large shop: search box carries the new placeholder", async () => {
    await renderList();
    expect(
      screen.getByPlaceholderText("Mã đơn hoặc tên sản phẩm"),
    ).toBeInTheDocument();
  });

  it("A non-seller principal is denied: no row and no ListSellerOrders call", async () => {
    vi.mocked(hasScope).mockReturnValue(false);
    const ui = await SellerOrdersPage({
      searchParams: Promise.resolve({}),
    });
    if (ui) render(ui);
    expect(screen.queryAllByRole("link", { name: "Chi tiết" })).toHaveLength(0);
    expect(listSellerOrdersPage).not.toHaveBeenCalled();
    expect(listSellerOrders).not.toHaveBeenCalled();
  });

  it("Search text stays out of the server logs: neither success nor failure logs the q value", async () => {
    const secret = "SECRET-needle-42";
    const spies = (["log", "info", "warn", "error", "debug"] as const).map(
      (m) => vi.spyOn(console, m).mockImplementation(() => {}),
    );
    await renderList({ q: secret });
    expect(lastRequest().query).toBe(secret);
    document.body.innerHTML = "";
    vi.mocked(listSellerOrdersPage).mockRejectedValue(new Error("down"));
    await renderList({ q: secret });
    const out = spies
      .flatMap((s) => s.mock.calls)
      .flat()
      .map(String)
      .join(" ");
    for (const s of spies) s.mockRestore();
    expect(out).not.toContain(secret);
  });

  it("has no brand-filled primary button", async () => {
    await renderList();
    expect(document.querySelectorAll(".bg-action-primary").length).toBe(0);
  });
});

describe("/seller/orders/[id]", () => {
  async function renderDetail(id = "o1", searchParams: { tab?: string } = {}) {
    const ui = await SellerOrderDetailPage({
      params: Promise.resolve({ id }),
      searchParams: Promise.resolve(searchParams),
    });
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

describe("/seller/orders bulk ship mount", () => {
  it("Bulk ship: only shippable rows can be selected: the page mounts checkboxes on PAID and COD rows and the bar", async () => {
    vi.mocked(listSellerOrdersPage).mockResolvedValue(
      pageOf([
        order("pa1", OrderStatus.PAID),
        order("cod1", OrderStatus.PENDING, {
          paymentMethod: PaymentMethod.COD,
        }),
        order("sh1", OrderStatus.SHIPPED),
      ]) as never,
    );
    await renderList();
    expect(
      screen.getByRole("checkbox", { name: "Chọn đơn #pa1" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: "Chọn đơn #cod1" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "Chọn đơn #sh1" })).toBeNull();
    expect(
      screen.getByRole("button", { name: "Gửi hàng loạt" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "In phiếu hàng loạt" }),
    ).toBeDisabled();
  });
});
