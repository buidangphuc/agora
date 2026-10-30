import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  redirect: (to: string) => {
    throw new Error(`NEXT_REDIRECT:${to}`);
  },
}));

import ShopProduct from "./[id]/page";
import ShopIndex from "./page";

describe("legacy shop routes", () => {
  it("sends /shop to the catalogue", () => {
    expect(() => ShopIndex()).toThrow("NEXT_REDIRECT:/products");
  });

  it("sends /shop/[id] to the product page", async () => {
    await expect(
      ShopProduct({ params: Promise.resolve({ id: "a b" }) }),
    ).rejects.toThrow("NEXT_REDIRECT:/products/a%20b");
  });
});
