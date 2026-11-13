import { render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { getStorefront, listMyListings } from "@/lib/gateway/listings";
import { listSellerOrders } from "@/lib/gateway/orders";
import { listMyCampaigns, listMyShopVouchers } from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";
import SellerLoading from "./loading";
import SellerPage from "./page";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn() }),
  usePathname: () => "/seller",
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(() => ({
    id: "s",
    name: "s",
    scopes: ["listing.write"],
  })),
  hasScope: vi.fn(),
}));
vi.mock("@/lib/gateway/listings", () => ({
  listMyListings: vi.fn(),
  getStorefront: vi.fn(),
}));
vi.mock("@/lib/gateway/promotion", () => ({
  listMyShopVouchers: vi.fn(),
  listMyCampaigns: vi.fn(),
}));
vi.mock("@/features/auth/guardPath", () => ({
  redirectGuestToLogin: vi.fn(() => {
    throw new Error("REDIRECT_TO_LOGIN");
  }),
}));
vi.mock("@/lib/gateway/orders", () => ({ listSellerOrders: vi.fn() }));
vi.mock("@/features/listing/actions", () => ({ deleteListingAction: vi.fn() }));

function listing(i: number, over: Record<string, unknown> = {}) {
  return {
    id: `listing-${String(i).padStart(3, "0")}`,
    title: `Sản phẩm ${i}`,
    price: 100000 * i,
    currency: "VND",
    status: "published",
    stock: 50,
    imageUrl: `https://img.test/${i}.png`,
    imageKeys: [],
    ...over,
  };
}

function orderRow(
  id: string,
  status: OrderStatus,
  extra: Record<string, unknown> = {},
) {
  return {
    id,
    status,
    paymentMethod: PaymentMethod.COD,
    shipBy: "",
    ...extra,
    statusText: "Chờ xử lý",
    recipientName: "An",
    totalAmount: 250000,
    createdAt: "01/10/2026",
  };
}

function givenListings(
  items: ReturnType<typeof listing>[],
  total = items.length,
) {
  vi.mocked(listMyListings).mockResolvedValue({
    items,
    nextCursor: "",
    total,
  } as never);
}

async function renderPage(searchParams: Record<string, string> = {}) {
  render(await SellerPage({ searchParams: Promise.resolve(searchParams) }));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(hasScope).mockReturnValue(true);
  vi.mocked(listSellerOrders).mockResolvedValue([]);
  givenListings([]);
  vi.mocked(getStorefront).mockResolvedValue({
    displayName: "Shop An",
    bannerUrl: "https://cdn/b.png",
  } as never);
  vi.mocked(listMyShopVouchers).mockResolvedValue({
    ok: true,
    items: [],
    nextCursor: "",
  } as never);
  vi.mocked(listMyCampaigns).mockResolvedValue({
    ok: true,
    items: [],
    nextCursor: "",
  } as never);
});

const kpiRow = () => screen.getByTestId("kpi-row");

describe("/seller Workplace", () => {
  it("shows KPIs derived from the seller's data", async () => {
    givenListings([listing(1), listing(2, { stock: 2 }), listing(3)]);
    vi.mocked(listSellerOrders).mockResolvedValue([
      orderRow("o1", OrderStatus.PENDING),
      orderRow("o2", OrderStatus.COMPLETED),
      orderRow("o3", OrderStatus.PENDING, {
        paymentMethod: PaymentMethod.MOCK_BANK,
      }),
    ] as never);
    await renderPage();
    const row = within(kpiRow());
    const cell = (title: string) =>
      row.getByText(title).closest("div") as HTMLElement;
    expect(cell("Tổng sản phẩm")).toHaveTextContent("3");
    expect(cell("Đang bán")).toHaveTextContent("3");
    expect(cell(/Sắp hết hàng/ as never)).toHaveTextContent("1");
    expect(cell("Cần gửi")).toHaveTextContent("1");
    expect(row.queryByText("Đơn chờ xử lý")).toBeNull();
  });

  it("[Order lifecycle: overdue shipments are highlighted] shows Quá hạn on the to-ship cell", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-10-17T10:00:00.000Z"));
    try {
      vi.mocked(listSellerOrders).mockResolvedValue([
        orderRow("o1", OrderStatus.PAID, {
          shipBy: "2026-10-17T09:00:00.000Z",
        }),
        orderRow("o2", OrderStatus.PAID, {
          shipBy: "2026-10-17T10:00:00.000Z",
        }),
      ] as never);
      await renderPage();
      const row = within(kpiRow());
      const cell = row.getByText("Cần gửi").closest("div") as HTMLElement;
      expect(cell).toHaveTextContent("Quá hạn: 1");
      expect(row.getAllByText(/Quá hạn: \d+/)).toHaveLength(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it("[Order lifecycle: overdue shipments are highlighted] no Quá hạn text without overdue orders", async () => {
    vi.mocked(listSellerOrders).mockResolvedValue([
      orderRow("o1", OrderStatus.PAID),
    ] as never);
    await renderPage();
    expect(within(kpiRow()).queryByText(/Quá hạn/)).toBeNull();
  });

  it("[Order lifecycle: the to-ship KPI is omitted when orders fail to load] to-ship KPI is omitted when orders fail", async () => {
    givenListings([listing(1)]);
    vi.mocked(listSellerOrders).mockRejectedValue(new Error("down"));
    await renderPage();
    expect(within(kpiRow()).queryByText("Cần gửi")).toBeNull();
    expect(screen.queryByText("Cần gửi")).toBeNull();
    expect(
      screen.getByText("Không tải được danh sách đơn hàng."),
    ).toBeInTheDocument();
  });

  it("hides the KPI whose source failed and keeps the others", async () => {
    givenListings([listing(1)]);
    vi.mocked(listSellerOrders).mockRejectedValue(new Error("down"));
    await renderPage();
    const row = within(kpiRow());
    expect(row.queryByText("Cần gửi")).toBeNull();
    expect(row.getByText("Tổng sản phẩm")).toBeInTheDocument();
    expect(
      screen.getByText("Không tải được danh sách đơn hàng."),
    ).toBeInTheDocument();
  });

  it("derives every value from the response, even when it is empty", async () => {
    await renderPage();
    const values = within(kpiRow())
      .getAllByText(/^\d+$/)
      .map((n) => n.textContent);
    expect(values).toEqual(["0", "0", "0", "0"]);
    // Zero comes from empty responses; the old fixed rose "0" card is gone.
    expect(screen.queryByText("Hết hàng / Tạm khóa")).toBeNull();
  });

  it("renders no KPI row when no source loaded", async () => {
    vi.mocked(listMyListings).mockRejectedValue(new Error("down"));
    vi.mocked(listSellerOrders).mockRejectedValue(new Error("down"));
    await renderPage();
    expect(screen.queryByTestId("kpi-row")).toBeNull();
    expect(
      screen.getByText("Không tải được số liệu tổng quan."),
    ).toBeInTheDocument();
  });

  it("quick actions link to the other seller areas; one brand CTA only", async () => {
    await renderPage();
    expect(screen.getByRole("link", { name: "Ví người bán" })).toHaveAttribute(
      "href",
      "/seller/wallet",
    );
    const primary = screen
      .getAllByRole("link")
      .filter((a) => a.className.includes("bg-action-primary"));
    expect(primary).toHaveLength(1);
    expect(primary[0]).toHaveTextContent("Thêm sản phẩm");
  });

  it("shows an Empty with a link when the seller has no orders", async () => {
    await renderPage();
    const empty = screen.getByText("Chưa có đơn hàng nào").closest("div");
    expect(
      within(empty as HTMLElement).getByRole("link", { name: "Thêm sản phẩm" }),
    ).toHaveAttribute("href", "/seller/new");
  });

  it("lists recent orders linking to their detail page", async () => {
    vi.mocked(listSellerOrders).mockResolvedValue([
      orderRow("order-abc12345", OrderStatus.PENDING),
    ] as never);
    await renderPage();
    expect(screen.getByRole("link", { name: "#order-ab" })).toHaveAttribute(
      "href",
      "/seller/orders/order-abc12345",
    );
  });

  it("shows the empty product state with a call to action", async () => {
    await renderPage();
    expect(screen.getByText("Shop chưa có sản phẩm")).toBeInTheDocument();
  });

  it("an Alert with retry replaces the table when the list read fails", async () => {
    vi.mocked(listMyListings).mockRejectedValue(new Error("boom"));
    await renderPage();
    expect(
      screen.getByText("Không tải được danh sách sản phẩm"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/boom/)).toBeNull();
    const retry = screen.getAllByRole("link", { name: "Thử lại" });
    expect(retry.some((a) => a.getAttribute("href") === "/seller")).toBe(true);
  });

  it("filters the page by q, with a clear-filter link when nothing matches", async () => {
    givenListings([listing(1), listing(2)]);
    await renderPage({ q: "Sản phẩm 2" });
    expect(screen.getAllByRole("link", { name: "Sản phẩm 2" })).toHaveLength(1);
    expect(screen.queryByRole("link", { name: "Sản phẩm 1" })).toBeNull();

    document.body.innerHTML = "";
    await renderPage({ q: "zzz" });
    expect(screen.getByText("Không có kết quả")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Xoá bộ lọc" })).toHaveAttribute(
      "href",
      "/seller",
    );
  });

  it("falls back to defaults for invalid page and status", async () => {
    givenListings([listing(1)]);
    await renderPage({ page: "abc", status: "bogus" });
    expect(listMyListings).toHaveBeenCalledWith({ cursor: "", pageSize: 20 });
    expect(screen.getByRole("link", { name: "Sản phẩm 1" })).toHaveAttribute(
      "href",
      "http://localhost:3000/listing/listing-001",
    );
  });

  it("marks the current page and links neighbours", async () => {
    vi.mocked(listMyListings)
      .mockResolvedValueOnce({
        items: [],
        nextCursor: "c1",
        total: 45,
      } as never)
      .mockResolvedValueOnce({
        items: [listing(21)],
        nextCursor: "c2",
        total: 45,
      } as never)
      .mockResolvedValue({
        items: [listing(1)],
        nextCursor: "",
        total: 45,
      } as never);
    await renderPage({ page: "2" });
    const nav = screen.getByRole("navigation", { name: "Phân trang" });
    expect(within(nav).getByRole("link", { name: "Trang 2" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(
      within(nav).getByRole("link", { name: "Trang trước" }),
    ).toHaveAttribute("href", "/seller");
    expect(
      within(nav).getByRole("link", { name: "Trang sau" }),
    ).toHaveAttribute("href", "/seller?page=3");
  });

  it("keeps thumbnails 1:1 and lazy after the first screen", async () => {
    givenListings(Array.from({ length: 8 }, (_, i) => listing(i + 1)));
    await renderPage();
    const imgs = screen.getAllByRole("img", { name: /^Sản phẩm \d$/ });
    expect(imgs).toHaveLength(8);
    expect(imgs[0]).toHaveAttribute("loading", "eager");
    expect(imgs[7]).toHaveAttribute("loading", "lazy");
    for (const img of imgs) {
      expect(img.parentElement).toHaveClass("aspect-square");
    }
  });

  it("never uses the native confirm() for delete", async () => {
    givenListings([listing(1)]);
    const confirm = vi.spyOn(window, "confirm");
    await renderPage();
    expect(
      screen.getByRole("button", { name: "Xoá Sản phẩm 1" }),
    ).toBeInTheDocument();
    expect(confirm).not.toHaveBeenCalled();
  });
});

describe("/seller loading skeleton", () => {
  it("renders the same KPI cells (and so heights) as the page", async () => {
    givenListings([listing(1)]);
    await renderPage();
    const pageRow = screen.getByTestId("kpi-row");
    const pageCells = Array.from(pageRow.children);

    document.body.innerHTML = "";
    render(<SellerLoading />);
    const skeletonRow = screen.getByTestId("kpi-row-skeleton");
    expect(skeletonRow.className).toBe(pageRow.className);
    expect(skeletonRow.children).toHaveLength(4);
    // Cells share the fixed-height Statistic structure (h-4 title, h-8 value).
    for (const cell of [...pageCells, ...Array.from(skeletonRow.children)]) {
      expect(cell.querySelector(".h-4")).not.toBeNull();
      expect(cell.querySelector(".h-8")).not.toBeNull();
    }
    expect(
      skeletonRow.querySelectorAll('[aria-busy="true"]').length,
    ).toBeGreaterThan(0);
  });
});

describe("/seller getting started card", () => {
  it("getting started uses the principal seller id", async () => {
    await renderPage({ sellerId: "s-other" });
    await waitFor(() => expect(getStorefront).toHaveBeenCalled());
    const ids = vi.mocked(getStorefront).mock.calls.map((c) => c[0]);
    expect(ids).toContain("s");
    expect(ids).not.toContain("s-other");
  });

  it("getting started reads each promotion and listing source with page size 1", async () => {
    await renderPage();
    await waitFor(() => expect(listMyShopVouchers).toHaveBeenCalled());
    expect(listMyShopVouchers).toHaveBeenCalledWith(
      expect.objectContaining({ pageSize: 1 }),
    );
    expect(listMyCampaigns).toHaveBeenCalledWith(
      expect.objectContaining({ pageSize: 1 }),
    );
    expect(listMyListings).toHaveBeenCalledWith(
      expect.objectContaining({ pageSize: 1 }),
    );
  });

  it("no card and no calls without seller scope", async () => {
    vi.mocked(getPrincipal).mockReturnValueOnce(null as never);
    await expect(
      SellerPage({ searchParams: Promise.resolve({}) }),
    ).rejects.toThrow("REDIRECT_TO_LOGIN");

    vi.mocked(hasScope).mockReturnValue(false);
    const ui = await SellerPage({ searchParams: Promise.resolve({}) });
    render(<>{ui}</>);
    expect(screen.queryByText("Bắt đầu bán hàng")).toBeNull();
    expect(getStorefront).not.toHaveBeenCalled();
    expect(listMyListings).not.toHaveBeenCalled();
    expect(listMyShopVouchers).not.toHaveBeenCalled();
    expect(listMyCampaigns).not.toHaveBeenCalled();
  });

  it("failed source shows chua ro and the dashboard stays", async () => {
    vi.mocked(listMyListings).mockImplementation(((input?: {
      pageSize?: number;
    }) =>
      input?.pageSize === 1
        ? Promise.reject(new Error("unavailable"))
        : Promise.resolve({
            items: [listing(1)],
            nextCursor: "",
            total: 1,
          })) as never);
    vi.mocked(listMyShopVouchers).mockResolvedValue({
      ok: true,
      items: [{ id: "v1" }],
      nextCursor: "",
    } as never);
    await renderPage();
    expect(await screen.findByText("Bắt đầu bán hàng")).toBeInTheDocument();
    const step = (await screen.findByText("chưa rõ")).closest("li");
    expect(step).toHaveAttribute("data-state", "unknown");
    expect(
      within(step as HTMLElement).getByRole("link", { name: "Thực hiện" }),
    ).toHaveAttribute("href", "/seller/new");
    expect(screen.getByTestId("kpi-row")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Sản phẩm 1" })).toBeInTheDocument();
  });

  it("a hanging source does not block the dashboard and the card sits above the KPI row", async () => {
    vi.mocked(getStorefront).mockReturnValue(new Promise(() => {}) as never);
    await renderPage();
    expect(screen.getByTestId("kpi-row")).toBeInTheDocument();
    expect(screen.getByTestId("getting-started-skeleton")).toBeInTheDocument();
    expect(
      screen
        .getByTestId("getting-started-skeleton")
        .compareDocumentPosition(screen.getByTestId("kpi-row")) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("loading skeleton includes the getting started card placeholder above the KPI row", () => {
    render(<SellerLoading />);
    const card = screen.getByTestId("getting-started-skeleton");
    expect(card.getAttribute("aria-busy")).toBe("true");
    expect(
      card.compareDocumentPosition(screen.getByTestId("kpi-row-skeleton")) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });
});
