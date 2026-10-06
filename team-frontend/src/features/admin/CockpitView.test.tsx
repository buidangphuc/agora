import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { type CockpitData, CockpitView } from "./CockpitView";

// Values the old view fabricated when data was missing. None may ever render
// from a missing/empty source.
const FABRICATED = [
  "485.2",
  "18.4",
  "1,420",
  "1420",
  "384,500,000",
  "384500000",
  "ord_d8f201",
  "ord_a71c99",
  "buyer_hcm@market.vn",
  "28,990,000",
  "4bf92f3577b3",
  "14%",
  "Flash sale",
  "10 Autonomous Services",
];

const eventSource = vi.fn();

function mockFetch(impl: () => Promise<Partial<Response>>) {
  vi.stubGlobal("fetch", vi.fn(impl));
}

function expectNoFabricated() {
  const text = document.body.textContent ?? "";
  for (const f of FABRICATED) expect(text).not.toContain(f);
}

beforeEach(() => {
  vi.stubGlobal("EventSource", eventSource);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  eventSource.mockClear();
});

const EMPTY: CockpitData = {
  timestamp: "2026-10-05T00:00:00Z",
  prometheus_available: false,
  total_rps: 0,
  avg_latency_ms: null,
  total_orders_24h: null,
  total_revenue_24h: null,
  services: [
    {
      name: "team-order",
      port: 50055,
      status: "UNKNOWN",
      rps: 0,
      p95_latency_ms: null,
      p99_latency_ms: null,
      error_rate: null,
    },
  ],
  recent_orders: [],
  recent_traces: [],
};

describe("CockpitView without data", () => {
  it("renders no hard-coded numbers before the first response", () => {
    mockFetch(() => new Promise(() => {}));
    render(<CockpitView initial={null} />);
    expectNoFabricated();
    const cards = screen.getByTestId("cockpit_metrics");
    expect(within(cards).getAllByText("—").length).toBeGreaterThanOrEqual(4);
  });

  it("shows an unavailable state when the gateway is unreachable", async () => {
    mockFetch(() => Promise.reject(new Error("network")));
    render(<CockpitView initial={null} />);
    await screen.findByText(/Không kết nối được gateway/);
    expectNoFabricated();
    expect(screen.getByText("Chưa có dữ liệu service.")).toBeTruthy();
  });

  it("shows no numbers when every source is down (server-fetched payload)", () => {
    mockFetch(() => new Promise(() => {}));
    render(<CockpitView initial={EMPTY} />);
    expectNoFabricated();
    const cards = screen.getByTestId("cockpit_metrics");
    expect(cards.textContent).not.toMatch(/\d+\.\d/);
    expect(cards.textContent).not.toMatch(/\d,\d{3}/);
    const health = screen.getByTestId("cockpit_health");
    const row = within(health).getByText("team-order").closest("tr");
    expect(row?.textContent).not.toMatch(/\d+(\.\d+)?\s*(ms|%)/);
    expect(
      within(screen.getByTestId("cockpit_orders")).getByText("Chưa có dữ liệu"),
    ).toBeTruthy();
    expect(screen.queryByTestId("cockpit_traces")).toBeNull();
    expect(screen.getAllByText(/Chưa có dữ liệu/).length).toBeGreaterThan(2);
    expect(screen.getByText(/Prometheus không khả dụng/)).toBeTruthy();
  });

  it("does not open the unused ops:orders SSE stream", () => {
    mockFetch(() => new Promise(() => {}));
    render(<CockpitView initial={EMPTY} />);
    expect(eventSource).not.toHaveBeenCalled();
    expect(screen.queryByText(/ops:orders/)).toBeNull();
    expect(screen.queryByText(/SSE/)).toBeNull();
  });

  it("says so when the session is rejected on refresh", async () => {
    mockFetch(() => Promise.resolve({ ok: false, status: 403 }));
    render(<CockpitView initial={null} />);
    await screen.findByText(/không có quyền admin/);
    expectNoFabricated();
  });
});

describe("CockpitView with real data", () => {
  const REAL: CockpitData = {
    ...EMPTY,
    prometheus_available: true,
    total_rps: 3.5,
    avg_latency_ms: 12.3,
    total_orders_24h: 7,
    total_revenue_24h: 2100000,
    services: [
      {
        name: "team-search",
        port: 50052,
        status: "HEALTHY",
        rps: 3.5,
        p95_latency_ms: 12.3,
        p99_latency_ms: 20,
        error_rate: 0.1,
      },
      {
        name: "team-chat",
        port: 50057,
        status: "IDLE",
        rps: 0,
        p95_latency_ms: null,
        p99_latency_ms: null,
        error_rate: null,
      },
    ],
    recent_orders: [
      {
        order_id: "ord-real-9",
        seller_id: "s-1",
        total: 300000,
        paid_at: "2026-10-05T08:30:15Z",
      },
    ],
    recent_traces: [
      {
        trace_id: "abc123def456",
        operation: "POST /api/orders",
        span_count: 4,
        duration_ms: 512.3,
        started_at: "2026-10-05T08:29:00Z",
        jaeger_url: "http://localhost:16686/trace/abc123def456",
      },
    ],
  };

  it("renders Prometheus values and honest per-row nulls", () => {
    mockFetch(() => new Promise(() => {}));
    render(<CockpitView initial={REAL} />);
    const cards = screen.getByTestId("cockpit_metrics");
    expect(cards.textContent).toContain("3.5");
    expect(cards.textContent).toContain("12.3");
    const health = screen.getByTestId("cockpit_health");
    expect(within(health).getByText("10.00%")).toBeTruthy();
    expectNoFabricated();
  });

  it("renders the order count, GMV, recent orders and traces from the payload", () => {
    mockFetch(() => new Promise(() => {}));
    render(<CockpitView initial={REAL} />);
    const cards = screen.getByTestId("cockpit_metrics");
    expect(cards.textContent).toContain("7");
    expect(cards.textContent).toContain("2,100,000");
    expect(cards.textContent).not.toContain("Chưa có dữ liệu");

    const orders = screen.getByTestId("cockpit_orders");
    expect(within(orders).getByText(/ord-real-9/)).toBeTruthy();
    expect(orders.textContent).toContain("300,000");
    expect(orders.textContent).toContain("08:30:15 UTC");

    const traces = screen.getByTestId("cockpit_traces");
    expect(within(traces).getByText("POST /api/orders")).toBeTruthy();
    expect(traces.textContent).toContain("4 spans");
    expect(traces.textContent).toContain("512.3 ms");
    expect(within(traces).getByRole("link").getAttribute("href")).toBe(
      "http://localhost:16686/trace/abc123def456",
    );
    expectNoFabricated();
  });

  it("renders a legitimate zero as 0, not as missing", () => {
    mockFetch(() => new Promise(() => {}));
    render(
      <CockpitView
        initial={{ ...REAL, total_orders_24h: 0, total_revenue_24h: 0 }}
      />,
    );
    const cards = screen.getByTestId("cockpit_metrics");
    expect(cards.textContent).toContain("0 đơn");
    expect(cards.textContent).not.toContain("Chưa có dữ liệu");
  });

  it("refreshes through the same-origin route handler, not the gateway", async () => {
    const fetchMock = vi.fn(() => new Promise(() => {}));
    vi.stubGlobal("fetch", fetchMock);
    render(<CockpitView initial={null} />);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/admin/metrics",
      expect.anything(),
    );
  });

  it("points external links at the real agora ports", () => {
    mockFetch(() => new Promise(() => {}));
    render(<CockpitView initial={EMPTY} />);
    const hrefs = screen
      .getAllByRole("link")
      .map((a) => a.getAttribute("href"));
    expect(hrefs).toContain("http://localhost:16686");
    expect(hrefs).toContain("http://localhost:8088");
    expect(hrefs).toContain("http://localhost:3001");
    expect(hrefs).toContain(
      "http://localhost:16686/search?service=team-gateway",
    );
  });
});
