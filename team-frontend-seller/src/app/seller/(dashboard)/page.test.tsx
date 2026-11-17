import { render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ListingStatus } from "@/generated/platform/listing/v1/listing_pb.js";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { getStorefront, listMyListings } from "@/lib/gateway/listings";
import { listSellerOrdersPage } from "@/lib/gateway/orders";
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
vi.mock("@/lib/gateway/orders", () => ({
  listSellerOrdersPage: vi.fn(),
}));
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

function givenOrders(
  orders: ReturnType<typeof orderRow>[],
  statusCounts: Partial<Record<OrderStatus, number>> = {},
) {
  vi.mocked(listSellerOrdersPage).mockResolvedValue({
    orders,
    nextCursor: "",
    total: orders.length,
    statusCounts,
  } as never);
}

async function renderPage(searchParams: Record<string, string> = {}) {
  render(await SellerPage({ searchParams: Promise.resolve(searchParams) }));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(hasScope).mockReturnValue(true);
  givenOrders([]);
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
  it("[KPIs of a large catalogue] shows counted totals and the to-ship count from status_counts", async () => {
    vi.mocked(listMyListings).mockImplementation((async (opts?: {
      statuses?: ListingStatus[];
      query?: string;
    }) => {
      const published =
        opts?.statuses?.includes(ListingStatus.PUBLISHED) && !opts?.query;
      return {
        items: Array.from({ length: 20 }, (_, i) => listing(i + 1)),
        nextCursor: "n1",
        total: published ? 1200 : 2000,
      };
    }) as never);
    givenOrders([orderRow("o1", OrderStatus.PENDING)], {
      [OrderStatus.PENDING]: 7,
      [OrderStatus.PAID]: 5,
      [OrderStatus.SHIPPED]: 300,
    });
    await renderPage();
    const row = within(kpiRow());
    const cell = (title: string) =>
      row.getByText(title).closest("div") as HTMLElement;
    expect(cell("Tổng sản phẩm")).toHaveTextContent("2.000");
    expect(cell("Đang bán")).toHaveTextContent("1.200");
    expect(cell("Cần gửi")).toHaveTextContent("12");
    expect(
      screen.getAllByRole("link", { name: /^Sản phẩm \d+$/ }),
    ).toHaveLength(20);
  });

  it("[KPIs of a large catalogue] makes one 5-row orders call and never walks listing cursors", async () => {
    givenListings([listing(1)], 2000);
    await renderPage();
    expect(listSellerOrdersPage).toHaveBeenCalledTimes(1);
    expect(listSellerOrdersPage).toHaveBeenCalledWith(
      expect.objectContaining({ pageSize: 5 }),
    );
    for (const [opts] of vi.mocked(listMyListings).mock.calls) {
      expect(opts?.cursor ?? "").toBe("");
    }
  });

  it("[Status filter runs in team-domain] status=draft sends statuses DRAFT and shows the returned rows", async () => {
    givenListings([
      listing(1, { status: "draft" }),
      listing(2, { status: "draft" }),
    ]);
    await renderPage({ status: "draft" });
    expect(listMyListings).toHaveBeenCalledWith(
      expect.objectContaining({
        pageSize: 20,
        statuses: [ListingStatus.DRAFT],
      }),
    );
    expect(
      screen.getByRole("link", { name: "Sản phẩm 1" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Sản phẩm 2" }),
    ).toBeInTheDocument();
  });

  it("[Title search runs in team-domain] q is sent as query and rows are not filtered again in the app", async () => {
    givenListings([listing(1, { title: "Quần jean" })]);
    await renderPage({ q: "áo thun" });
    expect(listMyListings).toHaveBeenCalledWith(
      expect.objectContaining({ pageSize: 20, query: "áo thun" }),
    );
    expect(screen.getByRole("link", { name: "Quần jean" })).toBeInTheDocument();
  });

  it("[Next product page uses the cursor] page 1 links Trang sau with the returned cursor and page=2", async () => {
    vi.mocked(listMyListings).mockResolvedValue({
      items: [listing(1)],
      nextCursor: "cur-2",
      total: 45,
    } as never);
    await renderPage();
    const nav = screen.getByRole("navigation", { name: "Phân trang" });
    const href = within(nav)
      .getByRole("link", { name: "Trang sau" })
      .getAttribute("href") as string;
    const url = new URL(href, "http://x");
    expect(url.pathname).toBe("/seller");
    expect(url.searchParams.get("cursor")).toBe("cur-2");
    expect(url.searchParams.get("page")).toBe("2");
  });

  it("[Next product page uses the cursor] opening cursor+page=2 requests that cursor once and shows other rows", async () => {
    vi.mocked(listMyListings).mockResolvedValue({
      items: [listing(21)],
      nextCursor: "",
      total: 45,
    } as never);
    await renderPage({ cursor: "cur-2", page: "2" });
    expect(listMyListings).toHaveBeenCalledWith(
      expect.objectContaining({ cursor: "cur-2", pageSize: 20 }),
    );
    expect(
      vi
        .mocked(listMyListings)
        .mock.calls.filter(([o]) => (o?.cursor ?? "") !== ""),
    ).toHaveLength(1);
    expect(
      screen.getByRole("link", { name: "Sản phẩm 21" }),
    ).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Phân trang" });
    expect(within(nav).getByText("Trang 2")).toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: "Trang sau" })).toBeNull();
  });

  it("[Orders source down degrades only its blocks] orders failure keeps the product table and listing KPIs", async () => {
    givenListings([listing(1)], 1);
    vi.mocked(listSellerOrdersPage).mockRejectedValue(new Error("down"));
    await renderPage();
    const row = within(kpiRow());
    expect(row.queryByText("Cần gửi")).toBeNull();
    expect(row.getByText("Tổng sản phẩm")).toBeInTheDocument();
    expect(
      screen.getByText("Không tải được danh sách đơn hàng."),
    ).toBeInTheDocument();
    expect(
      screen.getAllByRole("link", { name: "Thử lại" }).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getByRole("link", { name: "Sản phẩm 1" }),
    ).toBeInTheDocument();
  });

  it("[Listings source down shows the retry state] table failure shows the Alert and keeps the order blocks", async () => {
    vi.mocked(listMyListings).mockImplementation((async (opts?: {
      pageSize?: number;
    }) => {
      if (opts?.pageSize === 20) throw new Error("boom");
      return { items: [], nextCursor: "", total: 0 };
    }) as never);
    givenOrders([orderRow("order-abc12345", OrderStatus.PENDING)], {
      [OrderStatus.PENDING]: 1,
    });
    await renderPage();
    expect(
      screen.getByText("Không tải được danh sách sản phẩm"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/boom/)).toBeNull();
    expect(
      screen
        .getAllByRole("link", { name: "Thử lại" })
        .some((a) => a.getAttribute("href") === "/seller"),
    ).toBe(true);
    expect(screen.getByRole("link", { name: "#order-ab" })).toBeInTheDocument();
    expect(within(kpiRow()).getByText("Cần gửi")).toBeInTheDocument();
  });

  it("[An unknown product cursor shows the retry state] retry link goes to /seller without cursor", async () => {
    vi.mocked(listMyListings).mockImplementation((async (opts?: {
      cursor?: string;
    }) => {
      if (opts?.cursor === "not-a-cursor") throw new Error("bad cursor");
      return { items: [], nextCursor: "", total: 0 };
    }) as never);
    await renderPage({ cursor: "not-a-cursor" });
    expect(
      screen.getByText("Không tải được danh sách sản phẩm"),
    ).toBeInTheDocument();
    const retry = screen
      .getAllByRole("link", { name: "Thử lại" })
      .map((a) => a.getAttribute("href"));
    expect(retry).toContain("/seller");
    expect(retry.some((h) => h?.includes("cursor"))).toBe(false);
  });

  it("[A principal without the seller scope is denied] no rows and no listing or order call", async () => {
    vi.mocked(hasScope).mockReturnValue(false);
    const ui = await SellerPage({ searchParams: Promise.resolve({}) });
    render(ui);
    expect(screen.queryByRole("link", { name: /^Sản phẩm/ })).toBeNull();
    expect(screen.queryByTestId("kpi-row")).toBeNull();
    expect(listMyListings).not.toHaveBeenCalled();
    expect(listSellerOrdersPage).not.toHaveBeenCalled();
  });

  it("[Orders source down degrades only its blocks] hides every KPI and shows the notice when no source loaded", async () => {
    vi.mocked(listMyListings).mockRejectedValue(new Error("down"));
    vi.mocked(listSellerOrdersPage).mockRejectedValue(new Error("down"));
    await renderPage();
    expect(screen.queryByTestId("kpi-row")).toBeNull();
    expect(
      screen.getByText("Không tải được số liệu tổng quan."),
    ).toBeInTheDocument();
  });

  it("derives zero values from empty responses", async () => {
    await renderPage();
    const row = within(kpiRow());
    for (const title of ["Tổng sản phẩm", "Đang bán", "Cần gửi"]) {
      expect(
        (row.getByText(title).closest("div") as HTMLElement).textContent,
      ).toMatch(/0/);
    }
    expect(screen.queryByText("Hết hàng / Tạm khóa")).toBeNull();
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
    givenOrders([orderRow("order-abc12345", OrderStatus.PENDING)]);
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

  it("falls back to defaults for invalid page and status", async () => {
    givenListings([listing(1)]);
    await renderPage({ page: "abc", status: "bogus" });
    expect(listMyListings).toHaveBeenCalledWith(
      expect.objectContaining({ cursor: "", pageSize: 20, statuses: [] }),
    );
    expect(screen.getByRole("link", { name: "Sản phẩm 1" })).toHaveAttribute(
      "href",
      "http://localhost:3000/listing/listing-001",
    );
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
    render(ui);
    expect(screen.queryByText("Bắt đầu bán hàng")).toBeNull();
    expect(getStorefront).not.toHaveBeenCalled();
    expect(listMyListings).not.toHaveBeenCalled();
    expect(listSellerOrdersPage).not.toHaveBeenCalled();
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
    expect(
      screen.getByRole("link", { name: "Sản phẩm 1" }),
    ).toBeInTheDocument();
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
