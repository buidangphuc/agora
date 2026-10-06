import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Stepper } from "./Stepper";

const steps = [
  { id: 1, title: "Giỏ hàng", status: "complete" as const },
  { id: 2, title: "Thanh toán", status: "current" as const },
  { id: 3, title: "Hoàn tất", status: "upcoming" as const },
];

describe("Stepper", () => {
  it("marks only the current step with aria-current=step (horizontal)", () => {
    render(<Stepper steps={steps} />);
    const items = screen.getAllByRole("listitem");
    expect(items[1]).toHaveAttribute("aria-current", "step");
    expect(items[0]).not.toHaveAttribute("aria-current");
    expect(items[2]).not.toHaveAttribute("aria-current");
  });

  it("marks the current step in the vertical orientation too", () => {
    render(
      <Stepper
        orientation="vertical"
        steps={[
          ...steps.slice(0, 2),
          { id: 3, title: "Lỗi", status: "failed" as const },
        ]}
      />,
    );
    const items = screen.getAllByRole("listitem");
    expect(items[1]).toHaveAttribute("aria-current", "step");
    expect(screen.getByText("Lỗi")).toBeInTheDocument();
  });

  it("keeps titles available to assistive tech", () => {
    render(<Stepper steps={steps} />);
    expect(screen.getByText("Thanh toán")).toBeInTheDocument();
  });

  it("renders a step title as a link only when it has an href, inert when disabled", () => {
    render(
      <Stepper
        steps={[
          { id: 1, title: "Địa chỉ", status: "complete", href: "/c?step=a" },
          {
            id: 2,
            title: "Vận chuyển",
            status: "complete",
            href: "/c?step=b",
            disabled: true,
          },
          { id: 3, title: "Thanh toán", status: "current" },
        ]}
      />,
    );
    expect(screen.getByRole("link", { name: "Địa chỉ" })).toHaveAttribute(
      "href",
      "/c?step=a",
    );
    expect(screen.queryByRole("link", { name: "Vận chuyển" })).toBeNull();
    expect(screen.getByText("Vận chuyển")).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.queryByRole("link", { name: "Thanh toán" })).toBeNull();
  });
});
