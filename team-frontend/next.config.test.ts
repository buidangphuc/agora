// @vitest-environment node
import { describe, expect, it } from "vitest";

import nextConfig from "./next.config.mjs";

type Header = { key: string; value: string };
type Rule = { source: string; headers: Header[] };

async function rules(): Promise<Rule[]> {
  const fn = (nextConfig as { headers?: () => Promise<Rule[]> }).headers;
  expect(fn).toBeTypeOf("function");
  return (await (fn as () => Promise<Rule[]>)()) as Rule[];
}

const POLICY = "strict-origin-when-cross-origin";

describe("next.config headers", () => {
  // Scenario: Referrer hygiene: every response carries the policy header
  it("headers() sets Referrer-Policy and keeps CSP", async () => {
    const all = await rules();
    const global = all.filter((r) => r.source === "/:path*");
    expect(global.length).toBeGreaterThan(0);
    const headers = global.flatMap((r) => r.headers);
    const rp = headers.find((h) => h.key === "Referrer-Policy");
    expect(rp?.value).toBe(POLICY);
    const csp = headers.find((h) => h.key === "Content-Security-Policy");
    expect(csp).toBeDefined();
    expect(csp?.value).toContain("default-src 'self'");
  });

  // Scenario: Referrer hygiene: a weaker Referrer-Policy value is not configured
  it("weaker Referrer-Policy is not configured", async () => {
    const all = await rules();
    const entries = all
      .flatMap((r) => r.headers)
      .filter((h) => h.key.toLowerCase() === "referrer-policy");
    expect(entries).toHaveLength(1);
    expect(entries[0].value).toBe(POLICY);
    for (const weak of [
      "unsafe-url",
      "no-referrer-when-downgrade",
      "origin-when-cross-origin",
    ]) {
      expect(JSON.stringify(all)).not.toContain(`"${weak}"`);
      expect(entries[0].value).not.toBe(weak);
    }
  });
});
