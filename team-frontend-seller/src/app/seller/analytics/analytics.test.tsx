import { existsSync } from "node:fs";
import { resolve } from "node:path";
import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { AnalyticsView } from "@/features/seller/AnalyticsView";
import { redirect } from "next/navigation";

import {
  getDemandForecast,
  getRevenueBreakdown,
  getSellerFunnel,
} from "@/lib/gateway/analytics";
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
  getDemandForecast: vi.fn(),
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

const warmForecast = {
  sellerId: "me",
  listingId: "l1",
  dailyForecasts: [{ date: "2026-11-15", p10: 1, p50: 2.5, p90: 4 }],
  suggestedReorderPoint: 12.2,
  safetyStock: 3,
  modelVersion: "baseline-v1",
  isColdStart: false,
  generatedAt: "2026-11-13T18:30:00.000Z",
};

const sku = (n: number, rev: number) => ({
  sku: `SKU-${n}`,
  listingId: `l${n}`,
  revenue: rev,
  unitsSold: n,
});
const sections = () => screen.getAllByTestId("forecast-section");
const ERR = "Dự báo tạm thời không khả dụng";

async function renderPage(searchParams: { range?: string } = {}) {
  render(
    await SellerAnalyticsPage({ searchParams: Promise.resolve(searchParams) }),
  );
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
  vi.mocked(getDemandForecast).mockImplementation(
    async (sellerId, listingId) => ({
      ...warmForecast,
      sellerId,
      listingId,
    }),
  );
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

describe("/seller/analytics forecast card", () => {
  it("Card covers only the top five listings of the range: forecasts only the top five listings of the range", async () => {
    vi.mocked(getRevenueBreakdown).mockResolvedValue({
      days: revenue.days,
      topSkus: [
        sku(1, 100),
        sku(2, 700),
        sku(3, 300),
        sku(4, 600),
        sku(5, 200),
        sku(6, 500),
        sku(7, 400),
      ],
    });
    await renderPage();
    expect(getDemandForecast).toHaveBeenCalledTimes(5);
    expect(sections().map((e) => e.getAttribute("data-listing-id"))).toEqual([
      "l2",
      "l4",
      "l6",
      "l7",
      "l3",
    ]);
  });

  it("Range tab selects the candidate listings: range tab scopes the listing pick", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-11-15T03:00:00Z"));
    try {
      await renderPage({ range: "30d" });
    } finally {
      vi.useRealTimers();
    }
    const scoped = vi
      .mocked(getRevenueBreakdown)
      .mock.calls.map((c) => c[1])
      .filter((o) => o?.from !== undefined);
    expect(scoped).toHaveLength(1);
    const { from, to } = scoped[0] as { from: Date; to: Date };
    expect((to.getTime() - from.getTime()) / 86_400_000).toBe(30);
  });

  it("One listing fails, others render: one failed forecast does not hide the others", async () => {
    vi.mocked(getRevenueBreakdown).mockResolvedValue({
      days: revenue.days,
      topSkus: [sku(1, 300), sku(2, 200), sku(3, 100)],
    });
    vi.mocked(getDemandForecast).mockImplementation(
      async (sellerId, listingId) => {
        if (listingId === "l2") throw new Error("down");
        return { ...warmForecast, sellerId, listingId };
      },
    );
    await renderPage();
    const [a, b, c] = sections();
    expect(within(a).getByText(/Điểm đặt hàng lại/)).toBeInTheDocument();
    expect(within(b).getByText(ERR)).toBeInTheDocument();
    expect(within(b).queryByText(/Điểm đặt hàng lại/)).toBeNull();
    expect(within(c).getByText(/Điểm đặt hàng lại/)).toBeInTheDocument();
  });

  it("Forecast failure leaves funnel and revenue intact: forecast failure does not touch funnel and revenue", async () => {
    vi.mocked(getDemandForecast).mockRejectedValue(new Error("down"));
    await renderPage({ range: "30d" });
    expect(screen.getByTestId("kpi-row")).toBeInTheDocument();
    expect(screen.getAllByRole("progressbar")).toHaveLength(4);
    expect(
      screen.getByRole("table", { name: "Doanh thu theo ngày" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Không tải được dữ liệu phễu chuyển đổi."),
    ).toBeNull();
    expect(screen.queryByText("Không tải được dữ liệu doanh thu.")).toBeNull();
    expect(within(sections()[0]).getByText(ERR)).toBeInTheDocument();
  });

  it("Revenue read fails: revenue failure shows the forecast alert", async () => {
    vi.mocked(getRevenueBreakdown).mockImplementation(async (_id, opts) => {
      if (opts && "from" in opts && opts.from) throw new Error("down");
      return revenue;
    });
    await renderPage();
    expect(screen.getByText(ERR)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Thử lại" })).toHaveAttribute(
      "href",
      "/seller/analytics?range=7d",
    );
    expect(getDemandForecast).not.toHaveBeenCalled();
  });

  it("New seller without orders: no revenue means an empty forecast card", async () => {
    vi.mocked(getRevenueBreakdown).mockResolvedValue({ days: [], topSkus: [] });
    await renderPage();
    expect(screen.getByText("Chưa có đơn hàng để dự báo")).toBeInTheDocument();
    expect(getDemandForecast).not.toHaveBeenCalled();
  });

  it("Top SKU without a listing id is skipped: skips SKU rows without a listing id", async () => {
    vi.mocked(getRevenueBreakdown).mockResolvedValue({
      days: revenue.days,
      topSkus: [
        { sku: "ONLY-SKU", listingId: "", revenue: 900, unitsSold: 1 },
        sku(2, 100),
      ],
    });
    await renderPage();
    expect(getDemandForecast).toHaveBeenCalledTimes(1);
    expect(vi.mocked(getDemandForecast).mock.calls[0][1]).toBe("l2");
    expect(sections()).toHaveLength(1);
  });

  it("Query string cannot select another seller: ignores seller and listing ids from the URL", async () => {
    await renderPage({
      range: "30d",
      sellerId: "other",
      listingId: "other-1",
    } as { range?: string });
    expect(getDemandForecast).toHaveBeenCalled();
    for (const [sellerId, listingId] of vi.mocked(getDemandForecast).mock
      .calls) {
      expect(sellerId).toBe("me");
      expect(listingId).toBe("l1");
    }
  });

  it("Guest is redirected: guest triggers no forecast call", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(redirect).mockImplementation(() => {
      throw new Error("NEXT_REDIRECT");
    });
    await expect(
      SellerAnalyticsPage({ searchParams: Promise.resolve({}) }),
    ).rejects.toThrow("NEXT_REDIRECT");
    expect(getDemandForecast).not.toHaveBeenCalled();
  });

  it("Buyer without seller scope is denied: no listing.write scope triggers no forecast call", async () => {
    vi.mocked(hasScope).mockReturnValue(false);
    const out = await SellerAnalyticsPage({
      searchParams: Promise.resolve({}),
    });
    expect(out).toBeNull();
    expect(getDemandForecast).not.toHaveBeenCalled();
  });
});
