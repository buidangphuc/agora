import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Skeleton } from "./Skeleton";

describe("Skeleton", () => {
  it("is busy and announces loading without relying on visuals", () => {
    const { container } = render(<Skeleton />);
    expect(container.firstChild).toHaveAttribute("aria-busy", "true");
    expect(screen.getByText("Đang tải")).toBeInTheDocument();
  });

  it("text preset renders the requested number of lines", () => {
    const { container } = render(<Skeleton variant="text" lines={4} />);
    expect(container.querySelectorAll(".h-4")).toHaveLength(4);
  });

  it("image preset reserves its aspect box", () => {
    const { container } = render(<Skeleton variant="image" aspect="4/3" />);
    expect(container.querySelector(".aspect-4\\/3")).not.toBeNull();
  });

  it("avatar and card presets render", () => {
    const { container, rerender } = render(
      <Skeleton variant="avatar" size="lg" />,
    );
    expect(container.querySelector(".rounded-full")).not.toBeNull();
    rerender(<Skeleton variant="card" aspect="square" />);
    expect(container.querySelector(".aspect-square")).not.toBeNull();
  });
});
