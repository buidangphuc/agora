import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  redirect: (url: string) => {
    throw new Error(`NEXT_REDIRECT:${url}`);
  },
}));

import * as server from "./returnUrl";
import { loginPath, safeReturnUrl } from "./returnUrlShared";

const SAFE_TABLE: [string | null | undefined, string][] = [
  ["/listing/abc?variant=v2", "/listing/abc?variant=v2"],
  ["https://evil.example", "/"],
  ["//evil", "/"],
  ["/\\evil", "/"],
  ["javascript:alert(1)", "/"],
  ["/a%0d%0ab", "/"],
  ["/%2F%2Fevil", "/"],
  ["/a%20b", "/a%20b"],
  ["/login", "/"],
  ["/Register/Foo", "/"],
  ["/loginfoo", "/loginfoo"],
  ["", "/"],
  [undefined, "/"],
  [null, "/"],
  [`/${"a".repeat(2048)}`, "/"],
];

describe("returnUrlShared (client-safe module)", () => {
  it.each(SAFE_TABLE)(
    "[Absolute, protocol-relative and scheme URLs are rejected] safeReturnUrl(%j) -> %j",
    (raw, expected) => {
      expect(safeReturnUrl(raw)).toBe(expected);
    },
  );

  it("[loginPath encodes the return target] loginPath encodes and guards", () => {
    expect(loginPath("/listing/abc?variant=v2")).toBe(
      "/login?returnUrl=%2Flisting%2Fabc%3Fvariant%3Dv2",
    );
    expect(loginPath("//evil")).toBe("/login");
    expect(loginPath(undefined)).toBe("/login");
    expect(loginPath("/")).toBe("/login?returnUrl=%2F");
    expect(loginPath("/a%20b")).toBe("/login?returnUrl=%2Fa%2520b");
  });

  it("[loginPath encodes the return target] returnUrl.ts re-exports the same functions", () => {
    expect(server.safeReturnUrl).toBe(safeReturnUrl);
    expect(server.loginPath).toBe(loginPath);
  });

  it("[loginPath encodes the return target] source does not import server-only or next/navigation", () => {
    const src = readFileSync(resolve(__dirname, "returnUrlShared.ts"), "utf-8");
    expect(src).not.toMatch(/["']server-only["']/);
    expect(src).not.toMatch(/["']next\/navigation["']/);
  });
});
