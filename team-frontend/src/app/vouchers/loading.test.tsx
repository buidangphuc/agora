import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import VouchersLoading from "./loading";

describe("vouchers loading", () => {
  it("renders banner, scrollable tabs and a one-column (375px) card grid", () => {
    const { container } = render(<VouchersLoading />);
    expect(container.firstElementChild).toHaveAttribute("aria-busy", "true");
    const grid = container.querySelector(".grid") as HTMLElement;
    expect(grid).toHaveClass("grid-cols-1", "lg:grid-cols-2");
    expect(grid.children).toHaveLength(4);
    expect(container.querySelector(".overflow-x-auto")).not.toBeNull();
  });
});
