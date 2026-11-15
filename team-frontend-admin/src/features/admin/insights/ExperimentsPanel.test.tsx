import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { ExperimentsPanel } from "./ExperimentsPanel";

afterEach(cleanup);

// Contract under test:
//   <ExperimentsPanel result={ExperimentResult<GetExperimentReadoutResponse>}
//                     flag={string} rangeDays={7|14|30} />
//   `flag` is the raw text typed by the admin (may be ""); it prefills
//   <input name="flag"> in a <form method="get"> next to <select name="range">.
//   Root data-testid="insights_experiments".

const variant = (over: Record<string, unknown> = {}) => ({
  variant: "control",
  exposed: 200n,
  converted: 20n,
  rate: 0.1,
  ciLow: 0.066,
  ciHigh: 0.149,
  ...over,
});

const ok = (variants = [variant()], windowDays = 7) => ({
  status: "ok" as const,
  data: { variants, windowDays },
});

const ui = (result: unknown, flag = "home_feed_ranker", rangeDays = 7) =>
  render(
    <ExperimentsPanel
      result={result as never}
      flag={flag}
      rangeDays={rangeDays as never}
    />,
  );

describe("ExperimentsPanel", () => {
  // Scenario: Variant rows with interval
  it("renders variant rows with rate and interval", () => {
    ui(ok());
    const panel = screen.getByTestId("insights_experiments");
    for (const t of ["control", "200", "20", "10.0%", "6.6% – 14.9%"])
      expect(panel.textContent).toContain(t);
  });

  // Scenario: Conversion window is stated
  it("shows the conversion window", () => {
    ui(ok([variant()], 7));
    expect(screen.getByTestId("insights_experiments").textContent).toContain(
      "Cửa sổ chuyển đổi: 7 ngày",
    );
  });

  // Scenario: Interval is not recomputed
  it("renders the received interval", () => {
    ui(
      ok([
        variant({
          rate: 0.1,
          ciLow: 0.05,
          ciHigh: 0.2,
          exposed: 1000n,
          converted: 500n,
        }),
      ]),
    );
    const text = screen.getByTestId("insights_experiments").textContent ?? "";
    expect(text).toContain("5.0% – 20.0%");
    expect(text).toContain("10.0%");
    expect(text).not.toContain("50.0%");
  });

  it("offers a GET form with flag input and range selector", () => {
    ui(ok(), "home_feed_ranker", 30);
    const form = screen
      .getByTestId("insights_experiments")
      .querySelector("form");
    expect(form?.getAttribute("method")?.toLowerCase()).toBe("get");
    expect(
      (form?.querySelector('input[name="flag"]') as HTMLInputElement).value,
    ).toBe("home_feed_ranker");
    expect(
      (form?.querySelector('select[name="range"]') as HTMLSelectElement).value,
    ).toBe("30");
  });

  // Scenario: No flag entered
  it("prompts for a flag key when none is entered", () => {
    ui({ status: "no_flag" }, "");
    const panel = screen.getByTestId("insights_experiments");
    expect(panel.textContent).toContain("Nhập flag key để xem kết quả");
    expect(within(panel).queryAllByRole("table").length).toBe(0);
  });

  // Scenario: Malformed flag key is rejected
  it("rejects a malformed flag key", () => {
    const evil = "<img src=x onerror=alert(1)> bad";
    ui({ status: "bad_flag" }, evil);
    const panel = screen.getByTestId("insights_experiments");
    expect(panel.textContent).toContain("Flag key không hợp lệ");
    expect(panel.querySelector("img")).toBeNull();
    expect(within(panel).queryAllByRole("table").length).toBe(0);
    expect(
      (panel.querySelector('input[name="flag"]') as HTMLInputElement).value,
    ).toBe(evil);
  });

  // Scenario: Flag with no exposures
  it("no variants shows the empty state", () => {
    ui({ status: "empty" });
    const panel = screen.getByTestId("insights_experiments");
    expect(panel.textContent).toContain("Chưa có lượt hiển thị cho flag này");
    expect(within(panel).queryAllByRole("row").length).toBe(0);
  });

  // Scenario: Upstream rejects the request
  it("invalid shows a generic message", () => {
    ui({ status: "invalid" });
    const panel = screen.getByTestId("insights_experiments");
    expect(panel.textContent).toContain("Tham số không hợp lệ");
    expect(within(panel).queryAllByRole("table").length).toBe(0);
  });

  // Scenario: Report service unavailable
  it("unavailable shows a message without numbers", () => {
    ui({ status: "unavailable" });
    const panel = screen.getByTestId("insights_experiments");
    expect(panel.textContent).toContain("Tạm thời không có số liệu");
    expect(panel.textContent).not.toContain("%");
  });
});
