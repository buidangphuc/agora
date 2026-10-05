import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CockpitView } from "./CockpitView";

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

class FakeEventSource {
  static last: FakeEventSource | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  close = vi.fn();
  constructor(public url: string) {
    FakeEventSource.last = this;
  }
}

function mockFetch(impl: () => Promise<Partial<Response>>) {
  vi.stubGlobal("fetch", vi.fn(impl));
}

function expectNoFabricated() {
  const text = document.body.textContent ?? "";
  for (const f of FABRICATED) expect(text).not.toContain(f);
}

beforeEach(() => {
  vi.stubGlobal("EventSource", FakeEventSource);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  FakeEventSource.last = null;
});

describe("CockpitView without data", () => {
  it("renders no hard-coded numbers before the first response", () => {
    mockFetch(() => new Promise(() => {}));
    render(<CockpitView />);
    expectNoFabricated();
    const cards = screen.getByTestId("cockpit_metrics");
    expect(within(cards).getAllByText("—").length).toBeGreaterThanOrEqual(4);
  });

  it("shows an unavailable state when the gateway is unreachable", async () => {
    mockFetch(() => Promise.reject(new Error("network")));
    render(<CockpitView />);
    await screen.findByText(/Không kết nối được gateway/);
    expectNoFabricated();
    expect(screen.getByText("Chưa có dữ liệu service.")).toBeTruthy();
  });

  it("shows no numbers when Prometheus is down and orders/revenue are null", async () => {
    mockFetch(() =>
      Promise.resolve({
        ok: true,
        json: async () => ({
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
          recent_traces: [],
        }),
      }),
    );
    render(<CockpitView />);
    await screen.findByText(/Prometheus không khả dụng/);
    expectNoFabricated();
    const cards = screen.getByTestId("cockpit_metrics");
    expect(cards.textContent).not.toMatch(/\d+\.\d/);
    const health = screen.getByTestId("cockpit_health");
    const row = within(health).getByText("team-order").closest("tr");
    expect(row?.textContent).not.toMatch(/\d+(\.\d+)?\s*(ms|%)/);
    expect(screen.getAllByText(/Chưa có dữ liệu/).length).toBeGreaterThan(0);
  });

  it("shows an empty live-orders state and only real SSE orders", async () => {
    mockFetch(() => Promise.reject(new Error("network")));
    render(<CockpitView />);
    expect(
      screen.getByText("Chưa có đơn hàng nào được ghi nhận."),
    ).toBeTruthy();
    expect(FakeEventSource.last?.url).toContain("room=ops:orders");

    // An order event without an id is not rendered (no invented ids/amounts).
    FakeEventSource.last?.onmessage?.({
      data: JSON.stringify({ event: "OrderPlaced", data: {} }),
    });
    expect(
      screen.getByText("Chưa có đơn hàng nào được ghi nhận."),
    ).toBeTruthy();

    FakeEventSource.last?.onmessage?.({
      data: JSON.stringify({
        event: "OrderPlaced",
        data: { order_id: "ord_real_1", amount: 1000 },
      }),
    });
    await waitFor(() => expect(screen.getByText(/ord_real_1/)).toBeTruthy());
    expect(screen.queryByText(/buyer_/)).toBeNull();
  });
});

describe("CockpitView with real data", () => {
  it("renders Prometheus values and honest per-row nulls", async () => {
    mockFetch(() =>
      Promise.resolve({
        ok: true,
        json: async () => ({
          timestamp: "2026-10-05T00:00:00Z",
          prometheus_available: true,
          total_rps: 3.5,
          avg_latency_ms: 12.3,
          total_orders_24h: null,
          total_revenue_24h: null,
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
          recent_traces: [],
        }),
      }),
    );
    render(<CockpitView />);
    await screen.findByText("team-search");
    const cards = screen.getByTestId("cockpit_metrics");
    expect(cards.textContent).toContain("3.5");
    expect(cards.textContent).toContain("12.3");
    expect(cards.textContent).toContain("Chưa có dữ liệu");
    const health = screen.getByTestId("cockpit_health");
    expect(within(health).getByText("10.00%")).toBeTruthy();
    expectNoFabricated();
  });

  it("points external links at the real agora ports", async () => {
    mockFetch(() => Promise.reject(new Error("network")));
    render(<CockpitView />);
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
