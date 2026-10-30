import { afterEach, describe, expect, it, vi } from "vitest";

import { sellerAppUrl, sellerEntryUrl } from "./apps";

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

describe("sellerEntryUrl", () => {
  it("opens the seller login with next=/seller by default", () => {
    vi.stubEnv("NEXT_PUBLIC_SELLER_APP_URL", "");
    expect(sellerEntryUrl()).toBe("http://localhost:3001/login?next=%2Fseller");
  });

  it("encodes a deep seller path into next", () => {
    vi.stubEnv("NEXT_PUBLIC_SELLER_APP_URL", "https://seller.example.com");
    expect(sellerEntryUrl("/seller/orders?status=paid")).toBe(
      "https://seller.example.com/login?next=%2Fseller%2Forders%3Fstatus%3Dpaid",
    );
  });

  it("falls back to /seller for a next that is not a seller-app path", () => {
    vi.stubEnv("NEXT_PUBLIC_SELLER_APP_URL", "");
    for (const bad of ["https://evil.test/x", "//evil.test", "seller", ""]) {
      expect(sellerEntryUrl(bad), bad).toBe(
        "http://localhost:3001/login?next=%2Fseller",
      );
    }
  });
});
