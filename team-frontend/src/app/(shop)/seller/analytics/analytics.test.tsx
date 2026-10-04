import { existsSync } from "node:fs";
import { resolve } from "node:path";
import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { AnalyticsView } from "@/features/seller/AnalyticsView";
import { getRevenueBreakdown, getSellerFunnel } from "@/lib/gateway/analytics";
import { getPrincipal, hasScope } from "@/lib/gateway/session";
import SellerAnalyticsPage from "./page";

vi.mock("next/navigation", () => ({
  usePathname: () => "/seller/analytics",
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  hasScope: vi.fn(),
}));
vi.mock("@/lib/gateway/analytics", () => ({
  getSellerFunnel: vi.fn(),
  getRevenueBreakdown: vi.fn(),
}));

const iso = (daysAgo: number) =>
  new Date(Date.now() - daysAgo * 86_400_000).toISOString().slice(0, 10);

const funnel = { impressions: 1000, views: 400, adds: 100, orders: 25 };
const revenue = {
  days: [
    { day: iso(2), revenue: 500000, orderCount: 3 },
    { day: iso(20), revenue: 900000, orderCount: 5 },
    { day: iso(60), revenue: 1300000, orderCount: 8 },
  ],
  topSkus: [{ sku: "SKU-1", listingId: "l1", revenue: 500000, unitsSold: 4 }],
};

async function renderPage(searchParams: { range?: string } = {}) {
  render(await SellerAnalyticsPage({ searchParams }));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "me",
    name: "Me",
    scopes: ["listing.write"],
  });
  vi.mocked(hasScope).mockReturnValue(true);
  vi.mocked(getSellerFunnel).mockResolvedValue(funnel);
  vi.mocked(getRevenueBreakdown).mockResolvedValue(revenue);
});

describe("/seller/analytics", () => {
  it("range tabs are links; the active one is aria-current and the URL drives the data", async () => {
    await renderPage({ range: "30d" });
    const tabs = screen.getByRole("navigation", { name: "Tabs" });
    expect(within(tabs).getByRole("link", { name: "30 ngày" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(within(tabs).getByRole("link", { name: "90 ngày" })).toHaveAttribute(
      "href",
      "/seller/analytics?range=90d",
    );
    // 30d keeps the 2- and 20-day-old rows, drops the 60-day-old one.
    const table = screen.getByRole("table", { name: "Doanh thu theo ngày" });
    expect(within(table).getAllByRole("row")).toHaveLength(3); // header + 2
    expect(within(table).queryByText(iso(60))).toBeNull();
  });

  it("defaults to 7 days for an invalid range", async () => {
    await renderPage({ range: "bogus" });
    expect(screen.getByRole("link", { name: "7 ngày" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    const table = screen.getByRole("table", { name: "Doanh thu theo ngày" });
    expect(within(table).getAllByRole("row")).toHaveLength(2); // header + 1
  });

  it("shows the Statistic row, funnel Progress rows and totals from real data", async () => {
    await renderPage({ range: "90d" });
    const row = within(screen.getByTestId("kpi-row"));
    expect(row.getByText("Doanh thu")).toBeInTheDocument();
    expect(row.getByText("₫2.700.000")).toBeInTheDocument();
    expect(row.getByText("Tỷ lệ chuyển đổi")).toBeInTheDocument();
    expect(row.getByText("2.5%")).toBeInTheDocument();
    expect(screen.getAllByRole("progressbar")).toHaveLength(4);
    expect(
      screen.getByRole("progressbar", { name: "Đơn hàng" }),
    ).toHaveAttribute("aria-valuenow", "3");
  });

  it("a funnel failure shows an Alert with a retry link and the rest still renders", async () => {
    vi.mocked(getSellerFunnel).mockRejectedValue(new Error("down"));
    await renderPage({ range: "30d" });
    expect(
      screen.getByText("Không tải được dữ liệu phễu chuyển đổi."),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Thử lại" })).toHaveAttribute(
      "href",
      "/seller/analytics?range=30d",
    );
    expect(
      screen.getByRole("table", { name: "Doanh thu theo ngày" }),
    ).toBeInTheDocument();
    // Funnel-backed KPIs are hidden, revenue ones remain.
    const row = within(screen.getByTestId("kpi-row"));
    expect(row.queryByText("Lượt hiển thị")).toBeNull();
    expect(row.getByText("Doanh thu")).toBeInTheDocument();
  });

  it("an empty series shows an Empty", async () => {
    vi.mocked(getRevenueBreakdown).mockResolvedValue({ days: [], topSkus: [] });
    await renderPage();
    expect(
      screen.getByText("Chưa có doanh thu trong khoảng này"),
    ).toBeInTheDocument();
  });

  it("reads only the analytics gateway: the mock panel is gone", () => {
    for (const gone of [
      "features/seller/SellerAnalyticsMock.tsx",
      "features/seller/SellerFunnelPanel.tsx",
    ]) {
      expect(existsSync(resolve(__dirname, "../../..", gone))).toBe(false);
    }
    expect(getSellerFunnel).toBeDefined();
  });
});

describe("AnalyticsView", () => {
  it("hides conversion when there are no impressions", () => {
    render(
      <AnalyticsView
        funnel={{ impressions: 0, views: 0, adds: 0, orders: 0 }}
        revenue={{ days: [], topSkus: [] }}
        range="7d"
        retryHref="/x"
      />,
    );
    expect(screen.queryByText("Tỷ lệ chuyển đổi")).toBeNull();
    expect(screen.getByText("Chưa có dữ liệu phễu")).toBeInTheDocument();
  });
});
