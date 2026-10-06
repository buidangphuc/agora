import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { Descriptions } from "./Descriptions";

const items = [
  { key: "a", label: "Họ tên", children: "Nguyễn A" },
  { key: "b", label: "Trạng thái", children: "Đã xác minh" },
];

describe("Descriptions", () => {
  it("renders label/value pairs as a description list", () => {
    const { container } = render(<Descriptions title="KYC" items={items} />);
    expect(screen.getByRole("heading", { name: "KYC" })).toBeInTheDocument();
    expect(container.querySelectorAll("dt")).toHaveLength(2);
    expect(container.querySelector("dd")).toHaveTextContent("Nguyễn A");
  });

  it("loading renders skeleton rows instead of values", () => {
    const { container } = render(<Descriptions items={items} loading />);
    expect(screen.queryByText("Nguyễn A")).toBeNull();
    expect(container.querySelectorAll('[data-variant="text"]')).toHaveLength(2);
    expect(container.querySelector("[aria-busy='true']")).not.toBeNull();
  });

  it("empty items render an Empty block", () => {
    render(<Descriptions items={[]} emptyText="Chưa có thông tin" />);
    expect(screen.getByText("Chưa có thông tin")).toBeInTheDocument();
  });

  it("error renders an alert with a working retry", async () => {
    const onRetry = vi.fn();
    const user = setupUser();
    render(
      <Descriptions items={items} error="Tải thất bại" onRetry={onRetry} />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Tải thất bại");
    await user.click(screen.getByRole("button", { name: "Thử lại" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});
