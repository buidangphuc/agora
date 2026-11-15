import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { RetentionPanel } from "./RetentionPanel";

afterEach(cleanup);

// Contract under test: <RetentionPanel result={ReportResult<GetRetentionCohortsResponse>} weeks={4|12|26} />
// Root data-testid="insights_retention"; <form method="get"> with <select name="weeks">.

const cohort = (over: Record<string, unknown> = {}) => ({
  cohortWeek: "2026-11-02",
  cohortSize: 40n,
  d1: 0.5,
  d7: 0.25,
  d30: 0.1,
  repeatPurchase30d: 0.05,
  weeklyActive: [1, 0.6, 0.4],
  ...over,
});

const ok = (cohorts = [cohort()]) => ({ status: "ok" as const, data: { cohorts } });

describe("RetentionPanel", () => {
  // Scenario: Cohort rows
  it("renders cohort week, size and rates", () => {
    render(<RetentionPanel result={ok() as never} weeks={12} />);
    const panel = screen.getByTestId("insights_retention");
    for (const t of ["2026-11-02", "40", "50.0%", "25.0%", "10.0%", "5.0%"])
      expect(panel.textContent).toContain(t);
  });

  // Scenario: Weekly-active series is shown in order
  it("renders weekly active in order", () => {
    render(<RetentionPanel result={ok() as never} weeks={12} />);
    const text = screen.getByTestId("insights_retention").textContent ?? "";
    const a = text.indexOf("100.0%");
    const b = text.indexOf("60.0%");
    const c = text.indexOf("40.0%");
    expect(a).toBeGreaterThan(-1);
    expect(b).toBeGreaterThan(a);
    expect(c).toBeGreaterThan(b);
  });

  it("offers a GET form with the weeks selector", () => {
    render(<RetentionPanel result={ok() as never} weeks={26} />);
    const form = screen.getByTestId("insights_retention").querySelector("form");
    expect(form?.getAttribute("method")?.toLowerCase()).toBe("get");
    expect((form?.querySelector('select[name="weeks"]') as HTMLSelectElement).value).toBe("26");
  });

  // Scenario: No cohorts yet
  it("empty list shows the empty state", () => {
    render(<RetentionPanel result={{ status: "empty" } as never} weeks={12} />);
    const panel = screen.getByTestId("insights_retention");
    expect(panel.textContent).toContain("Chưa có nhóm đăng ký nào");
    expect(panel.textContent).not.toContain("%");
    expect(within(panel).queryAllByRole("row").length).toBe(0);
  });

  // Scenario: Retention report unavailable
  it("unavailable shows a message without numbers", () => {
    render(<RetentionPanel result={{ status: "unavailable" } as never} weeks={12} />);
    const panel = screen.getByTestId("insights_retention");
    expect(panel.textContent).toContain("Tạm thời không có số liệu");
    expect(panel.textContent).not.toContain("%");
    expect(within(panel).queryAllByRole("table").length).toBe(0);
  });
});
