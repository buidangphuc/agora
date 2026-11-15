import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { PlacementsPanel } from "./PlacementsPanel";

afterEach(cleanup);

// Contract under test (implementer creates PlacementsPanel.tsx):
//   <PlacementsPanel recommendation={R} search={R} windowHours={24|72|168} />
//   R = ReportResult<response> from "@/lib/gateway/insights"
//   Root element has data-testid="insights_placements"; selector is a
//   <form method="get"> with <select name="window">.

const row = (over: Record<string, unknown> = {}) => ({
  placementId: "home_feed",
  modelVersion: "g1/std/rank-a/arm-1",
  impressions: 1234n,
  itemImpressions: 9000n,
  clicks: 100n,
  addToCarts: 20n,
  purchases: 5n,
  ctr: 0.081,
  conversionRate: 0.05,
  ...over,
});

const rec = (rows = [row(), row({ modelVersion: "g1/std/rank-b/arm-2", impressions: 77n })]) => ({
  status: "ok" as const,
  data: {
    rows,
    fallback: [{ placementId: "home_feed", fallbackShare: 0.25 }],
    windowHours: 24,
    attributionWindowHours: 24,
  },
});

const search = () => ({
  status: "ok" as const,
  data: {
    rows: [row({ placementId: "search", modelVersion: "search-model-z", impressions: 4321n })],
    windowHours: 24,
    attributionWindowHours: 24,
  },
});

const ui = (p: Record<string, unknown> = {}) =>
  render(
    <PlacementsPanel
      recommendation={rec() as never}
      search={search() as never}
      windowHours={24}
      {...(p as object)}
    />,
  );

describe("PlacementsPanel", () => {
  // Scenario: Recommendation rows per model version
  it("renders one row per placement and model version verbatim", () => {
    ui();
    const panel = screen.getByTestId("insights_placements");
    expect(within(panel).getByText("g1/std/rank-a/arm-1")).toBeTruthy();
    expect(within(panel).getByText("g1/std/rank-b/arm-2")).toBeTruthy();
    expect(panel.textContent).toContain("1,234");
    expect(panel.textContent).toContain("8.1%");
    expect(panel.textContent).toContain("5.0%");
  });

  // Scenario: Search rows are shown in their own table
  it("renders search rows separately", () => {
    ui();
    const panel = screen.getByTestId("insights_placements");
    const tables = within(panel).getAllByRole("table");
    expect(tables).toHaveLength(2);
    expect(tables[0].textContent).toContain("g1/std/rank-a/arm-1");
    expect(tables[0].textContent).not.toContain("search-model-z");
    expect(tables[1].textContent).toContain("search-model-z");
    expect(tables[1].textContent).not.toContain("g1/std/rank-a/arm-1");
  });

  // Scenario: Fallback share is shown per placement
  it("renders fallback share as a percentage", () => {
    ui();
    const panel = screen.getByTestId("insights_placements");
    expect(panel.textContent).toContain("25.0%");
    expect(panel.textContent).toContain("home_feed");
  });

  // Scenario: Window selection is sent as hours (selector side)
  it("offers a GET form with the window selector and states the span", () => {
    ui({ windowHours: 168 });
    const panel = screen.getByTestId("insights_placements");
    const form = panel.querySelector("form");
    expect(form?.getAttribute("method")?.toLowerCase()).toBe("get");
    const select = form?.querySelector('select[name="window"]') as HTMLSelectElement;
    expect(select.value).toBe("168");
    expect(panel.textContent).toContain("7 ngày");
  });

  // Scenario: No impressions in the window
  it("empty result shows the empty state", () => {
    ui({ recommendation: { status: "empty" }, search: { status: "empty" } });
    const panel = screen.getByTestId("insights_placements");
    expect(panel.textContent).toContain("Chưa có dữ liệu trong khoảng này");
    expect(within(panel).queryAllByRole("row").length).toBe(0);
    expect(panel.textContent).not.toMatch(/0\.0%/);
  });

  // Scenario: Report service unavailable
  it("unavailable shows a message without numbers", () => {
    ui({ recommendation: { status: "unavailable" }, search: { status: "unavailable" } });
    const panel = screen.getByTestId("insights_placements");
    expect(panel.textContent).toContain("Tạm thời không có số liệu");
    expect(within(panel).queryAllByRole("table").length).toBe(0);
    expect(panel.textContent).not.toContain("%");
    expect(panel.textContent).not.toContain("1,234");
  });
});
