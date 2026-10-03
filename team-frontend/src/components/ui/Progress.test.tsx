import { render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Progress } from "./Progress";

describe("Progress", () => {
  it("is a progressbar exposing aria-valuenow, min and max", () => {
    render(<Progress percent={40} label="Tải lên" />);
    const bar = screen.getByRole("progressbar", { name: "Tải lên" });
    expect(bar).toHaveAttribute("aria-valuenow", "40");
    expect(bar).toHaveAttribute("aria-valuemin", "0");
    expect(bar).toHaveAttribute("aria-valuemax", "100");
    expect(document.querySelector("[style]")).toHaveStyle({ width: "40%" });
    expect(screen.getByText("40%")).toBeInTheDocument();
  });

  it("clamps out-of-range values", () => {
    const { rerender } = render(<Progress percent={150} />);
    expect(screen.getByRole("progressbar")).toHaveAttribute(
      "aria-valuenow",
      "100",
    );
    rerender(<Progress percent={-5} />);
    expect(screen.getByRole("progressbar")).toHaveAttribute(
      "aria-valuenow",
      "0",
    );
  });

  it("can hide the percentage text and uses status tokens", () => {
    const { container } = render(
      <Progress percent={100} status="success" showInfo={false} />,
    );
    expect(screen.queryByText("100%")).toBeNull();
    expect(container.querySelector(".bg-success")).not.toBeNull();
  });

  it("is server-renderable", () => {
    expect(renderToStaticMarkup(<Progress percent={10} />)).toContain(
      'aria-valuenow="10"',
    );
  });
});
