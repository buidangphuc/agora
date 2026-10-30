// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import nextConfig from "./next.config.mjs";

type Header = { key: string; value: string };
type Rule = { source: string; headers: Header[] };

async function rules(): Promise<Rule[]> {
  const fn = (nextConfig as { headers?: () => Promise<Rule[]> }).headers;
  expect(fn).toBeTypeOf("function");
  return (await (fn as () => Promise<Rule[]>)()) as Rule[];
}

describe("admin isolation headers", () => {
  it("sends X-Robots-Tag noindex, nofollow on every route", async () => {
    const all = await rules();
    const global = all.filter((r) => r.source === "/:path*");
    const robots = global
      .flatMap((r) => r.headers)
      .filter((h) => h.key === "X-Robots-Tag");
    expect(robots).toEqual([
      { key: "X-Robots-Tag", value: "noindex, nofollow" },
    ]);
  });

  it("keeps the strict referrer policy and forbids framing", async () => {
    const headers = (await rules()).flatMap((r) => r.headers);
    expect(headers.find((h) => h.key === "Referrer-Policy")?.value).toBe(
      "strict-origin-when-cross-origin",
    );
    expect(
      headers.find((h) => h.key === "Content-Security-Policy")?.value,
    ).toContain("frame-ancestors 'none'");
  });

  it("disallows every crawler in robots.txt", () => {
    const robots = readFileSync(join(__dirname, "public/robots.txt"), "utf8");
    expect(robots).toMatch(/^User-agent: \*$/m);
    expect(robots).toMatch(/^Disallow: \/$/m);
  });
});
