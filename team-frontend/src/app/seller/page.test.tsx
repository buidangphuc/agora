import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { listMyListings } from "@/lib/gateway/listings";
import { listSellerOrders } from "@/lib/gateway/orders";
import { hasScope } from "@/lib/gateway/session";
import SellerLoading from "./loading";
import SellerPage from "./page";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn() }),
  usePathname: () => "/seller",
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({ hasScope: vi.fn() }));
vi.mock("@/lib/gateway/listings", () => ({ listMyListings: vi.fn() }));
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

function orderRow(id: string, status: OrderStatus) {
  return {
    id,
    status,
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
  render(await SellerPage({ searchParams }));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(hasScope).mockReturnValue(true);
  vi.mocked(listSellerOrders).mockResolvedValue([]);
  givenListings([]);
});

const kpiRow = () => screen.getByTestId("kpi-row");

describe("/seller Workplace", () => {
  it("shows KPIs derived from the seller's data", async () => {
    givenListings([listing(1), listing(2, { stock: 2 }), listing(3)]);
    vi.mocked(listSellerOrders).mockResolvedValue([
      orderRow("o1", OrderStatus.PENDING),
      orderRow("o2", OrderStatus.COMPLETED),
    ] as never);
    await renderPage();
    const row = within(kpiRow());
    const cell = (title: string) =>
      row.getByText(title).closest("div") as HTMLElement;
    expect(cell("Tổng sản phẩm")).toHaveTextContent("3");
    expect(cell("Đang bán")).toHaveTextContent("3");
    expect(cell(/Sắp hết hàng/ as never)).toHaveTextContent("1");
    expect(cell("Đơn chờ xử lý")).toHaveTextContent("1");
  });

  it("hides the KPI whose source failed and keeps the others", async () => {
    givenListings([listing(1)]);
    vi.mocked(listSellerOrders).mockRejectedValue(new Error("down"));
    await renderPage();
    const row = within(kpiRow());
    expect(row.queryByText("Đơn chờ xử lý")).toBeNull();
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
      "/listing/listing-001",
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
    const skeletonRow = screen.getByTestId("kpi-row");
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
