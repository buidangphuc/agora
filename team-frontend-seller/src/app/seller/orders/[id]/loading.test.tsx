import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import SellerOrderDetailLoading from "./loading";

describe("Order page settlement card loading", () => {
  it("Breakdown of a settled order: loading skeleton includes the settlement card skeleton", () => {
    render(<SellerOrderDetailLoading />);
    expect(screen.getByTestId("settlement-skeleton")).toBeInTheDocument();
  });
});
