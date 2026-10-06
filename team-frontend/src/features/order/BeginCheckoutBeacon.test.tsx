import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { trackEcommerce } from "@/lib/analytics";

import { BeginCheckoutBeacon } from "./BeginCheckoutBeacon";

vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));

const items = [
  { itemId: "l1", itemName: "Áo", price: 100, quantity: 2, index: 1 },
];

describe("BeginCheckoutBeacon", () => {
  it("fires begin_checkout once with the cart items and value, not on re-render", () => {
    const { rerender } = render(
      <BeginCheckoutBeacon value={200} items={items} />,
    );
    // The page stays mounted across the four steps; each step re-renders it.
    for (let step = 0; step < 4; step++) {
      rerender(<BeginCheckoutBeacon value={200} items={[...items]} />);
    }
    expect(trackEcommerce).toHaveBeenCalledTimes(1);
    expect(trackEcommerce).toHaveBeenCalledWith("begin_checkout", {
      currency: "VND",
      value: 200,
      items,
    });
  });
});
