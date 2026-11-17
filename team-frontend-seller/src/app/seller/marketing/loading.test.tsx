import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import MarketingLoading from "./loading";

describe("/seller/marketing loading", () => {
  it("Seller opens the hub from the navigation: loading renders a busy skeleton", () => {
    render(<MarketingLoading />);
    expect(screen.getByTestId("seller-skeleton")).toHaveAttribute(
      "aria-busy",
      "true",
    );
  });
});
