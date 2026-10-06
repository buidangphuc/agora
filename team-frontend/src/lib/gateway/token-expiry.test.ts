import { describe, expect, it } from "vitest";

import { isUsableToken, tokenState } from "./token-expiry.js";

function jwt(payload: object): string {
  const b64 = (o: object) =>
    btoa(JSON.stringify(o))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "");
  return `${b64({ alg: "RS256" })}.${b64(payload)}.sig`;
}

const NOW = 1_800_000_000_000;
const sec = NOW / 1000;

describe("tokenState", () => {
  it("is usable before exp", () => {
    expect(tokenState(jwt({ exp: sec + 3600 }), NOW)).toBe("usable");
  });
  it("is expired at or after exp (with a few seconds of skew margin)", () => {
    expect(tokenState(jwt({ exp: sec - 1 }), NOW)).toBe("expired");
    expect(tokenState(jwt({ exp: sec + 2 }), NOW)).toBe("expired");
  });
  it("treats a token without exp as usable (the gateway decides)", () => {
    expect(tokenState(jwt({ sub: "u1" }), NOW)).toBe("usable");
  });
  it("flags garbage as malformed", () => {
    expect(tokenState("not.a.jwt", NOW)).toBe("malformed");
    expect(tokenState("garbage", NOW)).toBe("malformed");
    expect(isUsableToken("a.b.c", NOW)).toBe(false);
  });
});
