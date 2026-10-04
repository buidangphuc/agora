import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import CartError from "./error";
import CartLoading from "./loading";

describe("cart route states", () => {
  it("loading reserves a shop card, three item rows and the summary", () => {
    render(<CartLoading />);
    const root = screen.getByTestId("cart-skeleton");
    expect(root).toHaveAttribute("aria-busy", "true");
    // header + shop header + 3 rows x (image, text, price) + summary header + body
    expect(
      root.querySelectorAll("[data-variant]").length,
    ).toBeGreaterThanOrEqual(3 * 3 + 3);
    expect(root.querySelectorAll("li")).toHaveLength(3);
  });

  it("error shows an Alert whose Thử lại calls reset()", () => {
    const reset = vi.fn();
    render(<CartError error={new Error("x")} reset={reset} />);
    expect(screen.getByRole("alert")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
    expect(reset).toHaveBeenCalledTimes(1);
  });
});
