import { describe, expect, it } from "vitest";

import { SELLER_NAV, isNavActive } from "../nav";

describe("SELLER_NAV marketing entry", () => {
  it("Seller opens the hub from the navigation: Marketing links to /seller/marketing", () => {
    const item = SELLER_NAV.find((i) => i.label === "Marketing");
    expect(item?.href).toBe("/seller/marketing");
    expect(isNavActive("/seller/marketing", "/seller/marketing")).toBe(true);
  });
});
