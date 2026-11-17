import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { CockpitData } from "@/features/admin/CockpitView";
import { fetchCockpit } from "@/lib/gateway/cockpit";
import { fetchInsights, parseInsightParams } from "@/lib/gateway/insights";
import { getPrincipal, getToken } from "@/lib/gateway/session";
import CockpitPage from "./page";

vi.mock("next/navigation", () => ({
  redirect: vi.fn((url: string) => {
    throw new Error(`NEXT_REDIRECT:${url}`);
  }),
  notFound: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
}));
vi.mock("@/lib/gateway/cockpit", () => ({ fetchCockpit: vi.fn() }));
vi.mock("@/lib/gateway/insights", () => ({
  fetchInsights: vi.fn(),
  parseInsightParams: vi.fn(),
}));

const PARAMS = {
  windowHours: 24,
  weeks: 12,
  rangeDays: 7,
  flag: { kind: "none" },
} as const;

const insightsOf = (over: Record<string, unknown> = {}) => ({
  recommendation: { status: "empty" },
  search: { status: "empty" },
  retention: { status: "empty" },
  experiment: { status: "no_flag" },
  aiSurfaces: { status: "empty" },
  ...over,
});

const PANEL_IDS = [
  "insights_placements",
  "insights_retention",
  "insights_experiments",
  "insights_ai_surfaces",
];

const DATA: CockpitData = {
  timestamp: "2026-10-05T00:00:00Z",
  prometheus_available: true,
  total_rps: 1,
  avg_latency_ms: null,
  total_orders_24h: 7,
  total_revenue_24h: 2100000,
  services: [],
  recent_orders: [],
  recent_traces: [],
};

const admin = { id: "u-admin", name: "Admin", scopes: ["admin"] };

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise(() => {})),
  );
  vi.mocked(getPrincipal).mockReturnValue(admin);
  vi.mocked(getToken).mockReturnValue("tok");
  vi.mocked(fetchCockpit).mockResolvedValue({ status: "ok", data: DATA });
  vi.mocked(parseInsightParams).mockReturnValue(PARAMS as never);
  vi.mocked(fetchInsights).mockResolvedValue(insightsOf() as never);
});

describe("CockpitPage", () => {
  it("sends an anonymous visitor to /login without fetching", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    await expect(CockpitPage()).rejects.toThrow("NEXT_REDIRECT");
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Fadmin%2Fcockpit",
    );
    expect(fetchCockpit).not.toHaveBeenCalled();
  });

  it("sends an Unauthenticated-after-fetch visitor to /login with returnUrl", async () => {
    vi.mocked(fetchCockpit).mockResolvedValue({
      status: "unauthenticated",
    } as never);
    await expect(CockpitPage()).rejects.toThrow("NEXT_REDIRECT");
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Fadmin%2Fcockpit",
    );
  });

  it("renders a 403 result for a signed-in non-admin, without fetching", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-buyer",
      name: "Buyer",
      scopes: ["order.read"],
    });
    render(await CockpitPage());
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Quay lại trang chủ" }),
    ).toBeTruthy();
    expect(fetchCockpit).not.toHaveBeenCalled();
    expect(screen.queryByTestId("cockpit_metrics")).toBeNull();
  });

  it("fetches server-side with the session token and renders the metrics", async () => {
    render(await CockpitPage());
    expect(fetchCockpit).toHaveBeenCalledWith("tok");
    const cards = screen.getByTestId("cockpit_metrics");
    expect(cards.textContent).toContain("2,100,000");
    expect(redirect).not.toHaveBeenCalled();
  });

  it("renders the empty state when the gateway is unreachable", async () => {
    vi.mocked(fetchCockpit).mockResolvedValue({ status: "unavailable" });
    render(await CockpitPage());
    const cards = screen.getByTestId("cockpit_metrics");
    expect(cards.textContent).toContain("Chưa có dữ liệu");
    expect(cards.textContent).not.toMatch(/\d+ đơn|\d+ ₫/);
  });

  it("shows a 403 result when the gateway itself refuses the scope", async () => {
    vi.mocked(fetchCockpit).mockResolvedValue({ status: "forbidden" });
    render(await CockpitPage());
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
  });

  it("403 has no audit link (Non-admin cockpit visitor sees no audit link)", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-buyer",
      name: "Buyer",
      scopes: ["order.read"],
    });
    const { container } = render(await CockpitPage());
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(container.querySelector('a[href="/admin/audit"]')).toBeNull();
    expect(
      screen.queryByRole("link", { name: "Nhật ký kiểm toán" }),
    ).toBeNull();
  });

  it("non-admin sees no users link (Users UI: Non-admin cockpit visitor sees no users link)", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-buyer",
      name: "Buyer",
      scopes: ["order.read"],
    });
    const { container } = render(await CockpitPage());
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(container.querySelector('a[href="/admin/users"]')).toBeNull();
    expect(
      screen.queryByRole("link", { name: "Quản lý người dùng" }),
    ).toBeNull();
  });

  it("admin page contains the users link (Users UI: Cockpit shows the users link)", async () => {
    const { container } = render(await CockpitPage());
    const links = container.querySelectorAll('a[href="/admin/users"]');
    expect(links).toHaveLength(1);
    expect(links[0].textContent).toContain("Quản lý người dùng");
  });

  it("admin sees all four panels (Admin sees all four panels)", async () => {
    render(await CockpitPage({ searchParams: {} }));
    for (const id of PANEL_IDS) expect(screen.getByTestId(id)).toBeTruthy();
    expect(fetchInsights).toHaveBeenCalledWith(PARAMS);
  });

  it("non-admin renders 403 and calls no report RPC (Non-admin gets no report calls)", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-buyer",
      name: "Buyer",
      scopes: ["order.read"],
    });
    render(await CockpitPage({ searchParams: {} }));
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    for (const id of PANEL_IDS) expect(screen.queryByTestId(id)).toBeNull();
    expect(fetchInsights).not.toHaveBeenCalled();
  });

  it("gateway permission denied renders 403 (Gateway denies a report call)", async () => {
    vi.mocked(fetchInsights).mockResolvedValue(
      insightsOf({ retention: { status: "forbidden" } }) as never,
    );
    render(await CockpitPage({ searchParams: {} }));
    expect(fetchInsights).toHaveBeenCalled();
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    for (const id of PANEL_IDS) expect(screen.queryByTestId(id)).toBeNull();
  });

  it("guest is redirected to login (Anonymous visitor is redirected)", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    await expect(CockpitPage({ searchParams: {} })).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Fadmin%2Fcockpit",
    );
    expect(fetchInsights).not.toHaveBeenCalled();
  });

  it("report unauthenticated is redirected to login (Expired session is redirected)", async () => {
    vi.mocked(fetchInsights).mockResolvedValue(
      insightsOf({ aiSurfaces: { status: "unauthenticated" } }) as never,
    );
    await expect(CockpitPage({ searchParams: {} })).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Fadmin%2Fcockpit",
    );
  });
});
