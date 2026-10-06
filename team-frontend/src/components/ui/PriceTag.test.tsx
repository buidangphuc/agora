import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { PriceTag } from "./PriceTag";

describe("PriceTag", () => {
  it("shows the price", () => {
    const { container } = render(<PriceTag price={29990} />);
    expect(container).toHaveTextContent("29.990");
  });

  it("shows the strikethrough original and the rounded discount percent", () => {
    render(<PriceTag price={75000} originalPrice={100000} />);
    expect(screen.getByText("₫100.000")).toHaveClass("line-through");
    expect(screen.getByText("-25%")).toBeInTheDocument();
  });

  it("shows no discount when the original is not higher", () => {
    render(<PriceTag price={100000} originalPrice={100000} />);
    expect(screen.queryByText(/^-\d+%$/)).toBeNull();
  });

  it("supports every size and hiding the currency symbol", () => {
    for (const size of ["sm", "md", "lg", "xl"] as const) {
      const { container, unmount } = render(
        <PriceTag price={1000} size={size} showCurrencySymbol={false} />,
      );
      expect(container).toHaveTextContent("₫1.000");
      unmount();
    }
  });
});
