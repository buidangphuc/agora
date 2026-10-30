import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Spin } from "./Spin";

describe("Spin", () => {
  it("standalone exposes a status with an accessible label", () => {
    render(<Spin />);
    expect(screen.getByRole("status")).toHaveTextContent("Đang tải");
  });

  it("shows the tip text", () => {
    render(<Spin tip="Đang xử lý" />);
    expect(screen.getByRole("status")).toHaveTextContent("Đang xử lý");
  });

  it("wrapping children marks the region busy and keeps the content mounted", () => {
    const { container } = render(
      <Spin spinning>
        <p>nội dung</p>
      </Spin>,
    );
    expect(container.firstChild).toHaveAttribute("aria-busy", "true");
    expect(screen.getByText("nội dung")).toBeInTheDocument();
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  it("is idle when spinning is false", () => {
    const { container } = render(
      <Spin spinning={false}>
        <p>nội dung</p>
      </Spin>,
    );
    expect(container.firstChild).not.toHaveAttribute("aria-busy");
    expect(screen.queryByRole("status")).toBeNull();
  });
});
