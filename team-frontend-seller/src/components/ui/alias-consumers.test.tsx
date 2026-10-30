import { render, screen } from "@testing-library/react";
import React from "react";
import { describe, expect, it } from "vitest";

import { Button } from "./Button";
import { PriceTag } from "./PriceTag";

// The alias-override scenario (ui-design-tokens): overriding --color-action-primary
// restyles these two with no component change, which holds only while they read
// the alias rather than a Tier 1 primitive.
describe("components read Tier 2 aliases", () => {
  it("primary Button uses the action-primary alias", () => {
    render(<Button>Mua ngay</Button>);
    const cls = screen.getByRole("button", { name: "Mua ngay" }).className;
    expect(cls).toContain("bg-action-primary");
    expect(cls).not.toContain("bg-primary-500");
  });

  it("sale PriceTag uses the action-primary alias", () => {
    const { container } = render(
      <PriceTag price={90000} originalPrice={100000} />,
    );
    expect(container.querySelector(".text-action-primary")).not.toBeNull();
    expect(container.querySelector(".text-primary-500")).toBeNull();
  });
});
