// add-ga4-destination 1.2 — "Pseudonymous GA4 user_id" scenarios.
// Assumed interface: ga4UserId(principalId, key?) -> 64-char lowercase hex, or
// null when the key is unset/empty. `key` defaults to GA4_USER_ID_HMAC_KEY.
import { createHmac } from "node:crypto";

import { afterEach, describe, expect, it } from "vitest";

import { ga4UserId } from "./userId";

const ref = (id: string, key: string) =>
  createHmac("sha256", key).update(id).digest("hex");

afterEach(() => {
  delete process.env.GA4_USER_ID_HMAC_KEY;
});

describe("ga4UserId (Logged-in buyer with analytics consent gets a stable HMAC)", () => {
  it("equals the reference lowercase-hex HMAC-SHA256 of the id with the explicit key", () => {
    const out = ga4UserId("user-123", "k1");
    expect(out).toBe(ref("user-123", "k1"));
    expect(out).toMatch(/^[0-9a-f]{64}$/);
  });

  it("reads the key from GA4_USER_ID_HMAC_KEY by default", () => {
    process.env.GA4_USER_ID_HMAC_KEY = "env-key";
    expect(ga4UserId("user-123")).toBe(ref("user-123", "env-key"));
  });

  it("is stable across calls and differs per user and per key", () => {
    expect(ga4UserId("a", "k")).toBe(ga4UserId("a", "k"));
    expect(ga4UserId("a", "k")).not.toBe(ga4UserId("b", "k"));
    expect(ga4UserId("a", "k")).not.toBe(ga4UserId("a", "k2"));
  });

  it("does not embed the raw id in the output", () => {
    expect(ga4UserId("user-123", "k")).not.toContain("user-123");
  });
});

describe("ga4UserId (Missing key disables the feature)", () => {
  it("returns null when the env key is unset", () => {
    delete process.env.GA4_USER_ID_HMAC_KEY;
    expect(ga4UserId("user-123")).toBeNull();
  });

  it("returns null when the key is empty", () => {
    process.env.GA4_USER_ID_HMAC_KEY = "";
    expect(ga4UserId("user-123")).toBeNull();
    expect(ga4UserId("user-123", "")).toBeNull();
  });
});
