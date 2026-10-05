import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { CockpitData } from "@/features/admin/CockpitView";
import { fetchCockpit } from "@/lib/gateway/cockpit";
import { getPrincipal, getToken } from "@/lib/gateway/session";
import CockpitPage from "./page";

vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
}));
vi.mock("@/lib/gateway/cockpit", () => ({ fetchCockpit: vi.fn() }));

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
});

describe("CockpitPage", () => {
  it("sends an anonymous visitor to /login without fetching", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    expect(await CockpitPage()).toBeNull();
    expect(redirect).toHaveBeenCalledWith("/login");
    expect(fetchCockpit).not.toHaveBeenCalled();
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
});
