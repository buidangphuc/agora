import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Empty } from "./Empty";

describe("Empty", () => {
  it("renders a default illustration and description", () => {
    const { container } = render(<Empty />);
    expect(container.querySelector("svg")).not.toBeNull();
    expect(screen.getByText("Không có dữ liệu")).toBeInTheDocument();
  });

  it("renders a custom description, image and action", () => {
    render(
      <Empty
        image={<span data-testid-x="img">img</span>}
        description="Giỏ hàng trống"
        action={<a href="/">Mua sắm</a>}
      />,
    );
    expect(screen.getByText("Giỏ hàng trống")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Mua sắm" })).toBeInTheDocument();
    expect(screen.getByText("img")).toBeInTheDocument();
  });
});
