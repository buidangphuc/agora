import { cleanup, render, screen, within } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";

import { InsightsSection } from "./InsightsSection";

afterEach(cleanup);

// Contract under test:
//   <InsightsSection insights={Awaited<ReturnType<typeof fetchInsights>>}
//                    params={InsightParams} flagText={string} />
//   Renders the four panels (testids insights_placements, insights_retention,
//   insights_experiments, insights_ai_surfaces) from already-fetched results.

const params = {
  windowHours: 24,
  weeks: 12,
  rangeDays: 7,
  flag: { kind: "none" },
} as const;

const base = () => ({
  recommendation: {
    status: "ok",
    data: {
      rows: [
        {
          placementId: "home_feed",
          modelVersion: "g1/std/a/x",
          impressions: 10n,
          itemImpressions: 10n,
          clicks: 1n,
          addToCarts: 1n,
          purchases: 1n,
          ctr: 0.1,
          conversionRate: 0.1,
        },
      ],
      fallback: [{ placementId: "home_feed", fallbackShare: 0.25 }],
    },
  },
  search: { status: "empty" },
  retention: {
    status: "ok",
    data: {
      cohorts: [
        {
          cohortWeek: "2026-11-02",
          cohortSize: 40n,
          d1: 0.5,
          d7: 0.25,
          d30: 0.1,
          repeatPurchase30d: 0.05,
          weeklyActive: [1],
        },
      ],
    },
  },
  experiment: { status: "no_flag" },
  aiSurfaces: { status: "empty" },
});

const ui = (insights: unknown) => (
  <InsightsSection
    insights={insights as never}
    params={params as never}
    flagText=""
  />
);

const IDS = [
  "insights_placements",
  "insights_retention",
  "insights_experiments",
  "insights_ai_surfaces",
];

describe("InsightsSection", () => {
  // Scenario: Admin sees all four panels
  it("renders all four panels", () => {
    render(ui(base()));
    for (const id of IDS) expect(screen.getByTestId(id)).toBeTruthy();
  });

  // Scenario: Retention report unavailable
  it("one failing panel does not blank the others", () => {
    render(ui({ ...base(), retention: { status: "unavailable" } }));
    const retention = screen.getByTestId("insights_retention");
    expect(retention.textContent).toContain("Tạm thời không có số liệu");
    expect(retention.textContent).not.toContain("%");
    expect(screen.getByTestId("insights_placements").textContent).toContain(
      "g1/std/a/x",
    );
    expect(screen.getByTestId("insights_placements").textContent).toContain(
      "25.0%",
    );
    for (const id of IDS) expect(screen.getByTestId(id)).toBeTruthy();
    expect(
      within(screen.getByTestId("insights_ai_surfaces")).queryAllByRole("row")
        .length,
    ).toBe(0);
  });

  // Scenario: Bearer and upstream text never reach the browser
  it("serialized output has no token or upstream text", () => {
    const leaky = {
      status: "unavailable",
      message: "dial tcp 10.0.0.5:9000: connection refused",
      token: "Bearer test-token",
    };
    const html = renderToStaticMarkup(
      ui({
        ...base(),
        recommendation: leaky,
        search: leaky,
        retention: leaky,
        experiment: { ...leaky, status: "invalid" },
        aiSurfaces: leaky,
      }),
    );
    for (const id of IDS) expect(html).toContain(id);
    expect(html).toContain("Tạm thời không có số liệu");
    expect(html).not.toContain("10.0.0.5");
    expect(html).not.toContain("connection refused");
    expect(html).not.toContain("test-token");
    expect(html).not.toContain("Bearer");
  });
});
