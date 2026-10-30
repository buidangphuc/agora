import { afterEach, describe, expect, it, vi } from "vitest";

import { buyerAppUrl } from "./apps";

afterEach(() => vi.unstubAllEnvs());

describe("buyerAppUrl", () => {
  it("defaults to the local buyer app on port 3000", () => {
    vi.stubEnv("NEXT_PUBLIC_BUYER_APP_URL", "");
    expect(buyerAppUrl("/listing/l1")).toBe("http://localhost:3000/listing/l1");
  });

  it("uses NEXT_PUBLIC_BUYER_APP_URL without doubling slashes", () => {
    vi.stubEnv("NEXT_PUBLIC_BUYER_APP_URL", "https://shop.example.com/");
    expect(buyerAppUrl("chat")).toBe("https://shop.example.com/chat");
    expect(buyerAppUrl()).toBe("https://shop.example.com/");
  });
});
