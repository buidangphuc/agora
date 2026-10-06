import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { Timeline } from "./Timeline";

const items = [
  {
    key: "1",
    title: "Đã đặt hàng",
    time: "10:00",
    description: "Chờ xác nhận",
    tone: "success" as const,
  },
  { key: "2", title: "Đang giao", tone: "primary" as const, current: true },
];

describe("Timeline", () => {
  it("renders an ordered list of events with descriptions and times", () => {
    render(<Timeline items={items} />);
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(screen.getByText("10:00")).toBeInTheDocument();
    expect(screen.getByText("Chờ xác nhận")).toBeInTheDocument();
  });

  it("marks the current event", () => {
    render(<Timeline items={items} />);
    expect(screen.getAllByRole("listitem")[1]).toHaveAttribute(
      "aria-current",
      "step",
    );
  });

  it("puts an item's testId on its list item", () => {
    render(<Timeline items={[{ key: "1", title: "A", testId: "evt" }]} />);
    expect(screen.getByTestId("evt").tagName).toBe("LI");
  });

  it("loading shows a skeleton and is busy", () => {
    const { container } = render(<Timeline items={items} loading />);
    expect(screen.queryByText("Đã đặt hàng")).toBeNull();
    expect(container.querySelector("[aria-busy='true']")).not.toBeNull();
  });

  it("empty shows an Empty block", () => {
    render(<Timeline items={[]} emptyText="Chưa có cập nhật" />);
    expect(screen.getByText("Chưa có cập nhật")).toBeInTheDocument();
  });

  it("error shows an alert with retry", async () => {
    const user = setupUser();
    const onRetry = vi.fn();
    render(<Timeline items={items} error="Lỗi mạng" onRetry={onRetry} />);
    expect(screen.getByRole("alert")).toHaveTextContent("Lỗi mạng");
    await user.click(screen.getByRole("button", { name: "Thử lại" }));
    expect(onRetry).toHaveBeenCalled();
  });
});
