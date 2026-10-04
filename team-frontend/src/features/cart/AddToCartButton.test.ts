import { describe, expect, it } from "vitest";

import { AddToCartButton } from "./AddToCartButton";
import { PurchasePanel } from "./PurchasePanel";

describe("AddToCartButton", () => {
  it("is a thin alias of PurchasePanel for existing imports", () => {
    expect(AddToCartButton).toBe(PurchasePanel);
  });
});
