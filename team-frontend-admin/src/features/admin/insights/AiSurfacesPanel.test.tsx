import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { AiSurfacesPanel } from "./AiSurfacesPanel";

afterEach(cleanup);

// Contract under test: <AiSurfacesPanel result={ReportResult<GetAiSurfaceReportResponse>} rangeDays={7|14|30} />
// Root data-testid="insights_ai_surfaces"; <form method="get"> with <select name="range">.

const row = (over: Record<string, unknown> = {}) => ({
  surface: "assistant",
  modelVersion: "mock:v1|hybrid-v2",
  shown: 10n,
  accepted: 4n,
  edited: 1n,
  dismissed: 2n,
  ...over,
});

const ok = (rows = [row()]) => ({ status: "ok" as const, data: { rows } });

describe("AiSurfacesPanel", () => {
  // Scenario: Surface rows with shares
  it("renders surface rows with accepted and dismissed shares", () => {
    render(<AiSurfacesPanel result={ok() as never} rangeDays={7} />);
    const panel = screen.getByTestId("insights_ai_surfaces");
    expect(within(panel).getByText("assistant")).toBeTruthy();
    expect(within(panel).getByText("mock:v1|hybrid-v2")).toBeTruthy();
    expect(panel.textContent).toContain("40.0%");
    expect(panel.textContent).toContain("20.0%");
  });

  // Scenario: Zero shown has no share
  it("zero shown renders a dash share", () => {
    render(
      <AiSurfacesPanel
        result={
          ok([
            row({ shown: 0n, accepted: 0n, edited: 0n, dismissed: 3n }),
          ]) as never
        }
        rangeDays={7}
      />,
    );
    const panel = screen.getByTestId("insights_ai_surfaces");
    expect(panel.textContent).not.toContain("%");
    expect(panel.textContent).not.toContain("Infinity");
    expect(panel.textContent).not.toContain("NaN");
    expect(panel.textContent).toContain("—");
  });

  it("offers a GET form with the range selector", () => {
    render(<AiSurfacesPanel result={ok() as never} rangeDays={14} />);
    const form = screen
      .getByTestId("insights_ai_surfaces")
      .querySelector("form");
    expect(form?.getAttribute("method")?.toLowerCase()).toBe("get");
    expect(
      (form?.querySelector('select[name="range"]') as HTMLSelectElement).value,
    ).toBe("14");
  });

  // Scenario: No AI interactions in range
  it("empty result shows the empty state", () => {
    render(
      <AiSurfacesPanel result={{ status: "empty" } as never} rangeDays={7} />,
    );
    const panel = screen.getByTestId("insights_ai_surfaces");
    expect(panel.textContent).toContain(
      "Chưa có tương tác AI trong khoảng này",
    );
    expect(within(panel).queryAllByRole("row").length).toBe(0);
  });

  // Scenario: AI report unavailable
  it("unavailable shows a message without numbers", () => {
    render(
      <AiSurfacesPanel
        result={{ status: "unavailable" } as never}
        rangeDays={7}
      />,
    );
    const panel = screen.getByTestId("insights_ai_surfaces");
    expect(panel.textContent).toContain("Tạm thời không có số liệu");
    expect(panel.textContent).not.toContain("%");
    expect(within(panel).queryAllByRole("table").length).toBe(0);
  });
});
