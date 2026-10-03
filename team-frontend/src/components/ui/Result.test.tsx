import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Result } from "./Result";

describe("Result", () => {
  it("renders title, subtitle, details and extra", () => {
    render(
      <Result
        status="success"
        title="Đặt hàng thành công"
        subTitle="Mã đơn 123"
        extra={<a href="/orders">Xem đơn</a>}
      >
        <p>Chi tiết</p>
      </Result>,
    );
    expect(
      screen.getByRole("heading", { name: "Đặt hàng thành công" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Mã đơn 123")).toBeInTheDocument();
    expect(screen.getByText("Chi tiết")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Xem đơn" })).toBeInTheDocument();
  });

  it("announces politely and names its status icon", () => {
    const { container } = render(
      <Result status="error" title="Thanh toán lỗi" />,
    );
    expect(container.firstChild).toHaveAttribute("aria-live", "polite");
    expect(screen.getByRole("img", { name: "Lỗi" })).toBeInTheDocument();
  });

  it("supports every status", () => {
    for (const status of [
      "success",
      "error",
      "info",
      "warning",
      "404",
    ] as const) {
      const { unmount } = render(<Result status={status} title={status} />);
      expect(screen.getByRole("heading", { name: status })).toBeInTheDocument();
      unmount();
    }
  });
});
