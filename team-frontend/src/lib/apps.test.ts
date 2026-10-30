import { afterEach, describe, expect, it, vi } from "vitest";

import { sellerAppUrl } from "./apps";

afterEach(() => vi.unstubAllEnvs());

describe("sellerAppUrl", () => {
  it("defaults to the seller centre of the local seller app on port 3001", () => {
    vi.stubEnv("NEXT_PUBLIC_SELLER_APP_URL", "");
    expect(sellerAppUrl()).toBe("http://localhost:3001/seller");
  });

  it("uses NEXT_PUBLIC_SELLER_APP_URL without doubling slashes", () => {
    vi.stubEnv("NEXT_PUBLIC_SELLER_APP_URL", "https://seller.example.com/");
    expect(sellerAppUrl("seller/new")).toBe(
      "https://seller.example.com/seller/new",
    );
  });
});
