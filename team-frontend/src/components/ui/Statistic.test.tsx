import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Statistic } from "./Statistic";

describe("Statistic", () => {
  it("renders title, value, prefix, suffix and trend", () => {
    render(
      <Statistic
        title="Doanh thu"
        value="1.200"
        prefix="₫"
        suffix="tr"
        trend={{ value: "5%", isUp: true, label: "so với tuần trước" }}
      />,
    );
    expect(screen.getByText("Doanh thu")).toBeInTheDocument();
    expect(screen.getByText("1.200")).toBeInTheDocument();
    expect(screen.getByText("↑")).toBeInTheDocument();
    expect(screen.getByText("so với tuần trước")).toBeInTheDocument();
  });

  it("a loading statistic keeps the same fixed-height rows as the loaded one", () => {
    const trend = { value: "5%", isUp: false };
    const loadedRender = render(
      <Statistic title="Đơn" value="12" trend={trend} />,
    );
    // Value row (h-8) and trend row (h-4) are fixed-height wrappers.
    const rowsOf = (root: HTMLElement) =>
      Array.from(
        root.querySelectorAll<HTMLElement>(".mt-2.h-8, .mt-2\\.5.h-4"),
      ).map((el) => el.className);
    const loadedRows = rowsOf(loadedRender.container);
    loadedRender.unmount();

    const loading = render(
      <Statistic title="Đơn" value="12" trend={trend} loading />,
    );
    expect(screen.queryByText("12")).toBeNull();
    expect(loading.container.firstChild).toHaveAttribute("aria-busy", "true");
    expect(rowsOf(loading.container)).toEqual(loadedRows);
    expect(loadedRows).toHaveLength(2);
  });

  it("shows an error alert in place of the value", () => {
    render(<Statistic title="Đơn" value="12" error="Lỗi tải" />);
    expect(screen.getByRole("alert")).toHaveTextContent("Lỗi tải");
    expect(screen.queryByText("12")).toBeNull();
  });
});
