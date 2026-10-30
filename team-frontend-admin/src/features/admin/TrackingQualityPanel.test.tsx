import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { type CockpitData, CockpitView } from "./CockpitView";
import {
  type TrackingQuality,
  TrackingQualityPanel,
} from "./TrackingQualityPanel";

afterEach(cleanup);

const DEGRADED: TrackingQuality = {
  status: "DEGRADED",
  reasons: ["stale", "incomplete"],
  last_ingested_at: "2026-10-09T01:02:03Z",
  lag_p50_seconds: 1.5,
  lag_p95_seconds: 42,
  decode_failures: 3,
  duplicates_skipped: 7,
  types: [
    {
      event_type: "view",
      events: 1234,
      visitors: 56,
      missing_listing_ratio: 0.25,
      listing_scoped: true,
    },
    {
      event_type: "search",
      events: 9,
      visitors: 4,
      missing_listing_ratio: 0,
      listing_scoped: false,
    },
  ],
};

describe("TrackingQualityPanel", () => {
  it("renders status, reasons, last ingest time and per-type rows", () => {
    render(<TrackingQualityPanel quality={DEGRADED} />);
    const panel = screen.getByTestId("tracking-quality-panel");
    expect(within(panel).getByText("Tracking data quality")).toBeTruthy();
    expect(screen.getByTestId("tracking-quality-status").textContent).toBe(
      "DEGRADED",
    );
    expect(within(panel).getByText("stale")).toBeTruthy();
    expect(within(panel).getByText("incomplete")).toBeTruthy();
    expect(panel.textContent).toContain("2026-10-09 01:02:03 UTC");
    const view = screen.getByTestId("tracking-quality-row-view");
    expect(view.textContent).toContain("1,234");
    expect(view.textContent).toContain("56");
    expect(view.textContent).toContain("25.0%");
    const search = screen.getByTestId("tracking-quality-row-search");
    expect(search.textContent).toContain("9");
    expect(search.textContent).not.toContain("0.0%");
    expect(panel.textContent).not.toMatch(/unavailable/i);
  });

  it("shows OK without reasons", () => {
    render(
      <TrackingQualityPanel
        quality={{ ...DEGRADED, status: "OK", reasons: [] }}
      />,
    );
    expect(screen.getByTestId("tracking-quality-status").textContent).toBe(
      "OK",
    );
    expect(screen.queryByTestId("tracking-quality-reasons")).toBeNull();
  });

  it("handles no ingest yet and no types", () => {
    render(
      <TrackingQualityPanel
        quality={{ ...DEGRADED, last_ingested_at: null, types: [] }}
      />,
    );
    expect(screen.queryByTestId("tracking-quality-row-view")).toBeNull();
    expect(screen.getByText(/No tracking events/)).toBeTruthy();
  });

  it.each([null, undefined])(
    "says unavailable and shows no counts when the section is %s",
    (q) => {
      render(<TrackingQualityPanel quality={q} />);
      const panel = screen.getByTestId("tracking-quality-panel");
      expect(within(panel).getByText("Tracking data quality")).toBeTruthy();
      expect(panel.textContent).toContain("unavailable");
      expect(panel.textContent).not.toMatch(/\d/);
      expect(screen.queryByTestId("tracking-quality-status")).toBeNull();
      expect(screen.queryByRole("table")).toBeNull();
    },
  );

  it("is rendered by the cockpit from the snapshot", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise(() => {})),
    );
    const data: CockpitData = {
      timestamp: "2026-10-09T00:00:00Z",
      total_rps: 0,
      avg_latency_ms: null,
      total_orders_24h: null,
      total_revenue_24h: null,
      services: [],
      recent_orders: [],
      recent_traces: [],
      tracking_quality: DEGRADED,
    };
    render(<CockpitView initial={data} />);
    expect(screen.getByTestId("tracking-quality-row-view")).toBeTruthy();
    vi.unstubAllGlobals();
  });
});
