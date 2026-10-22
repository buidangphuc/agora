// @vitest-environment node
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const root = resolve(__dirname, "../..");
const envExample = readFileSync(resolve(root, ".env.example"), "utf8");
const readme = readFileSync(resolve(root, "README.md"), "utf8");

describe("Cookie secure docs (task 1.5)", () => {
  it("Cookie secure: local compose leaves it unset: .env.example has COOKIE_SECURE only as a comment", () => {
    const lines = envExample.split("\n");
    expect(lines.some((l) => /^#\s*COOKIE_SECURE=true\s*$/.test(l))).toBe(true);
    expect(lines.some((l) => /^\s*COOKIE_SECURE=/.test(l))).toBe(false);
  });

  it("Cookie secure: local compose leaves it unset: .env.example notes server-only and TLS-only", () => {
    const i = envExample.indexOf("COOKIE_SECURE");
    expect(i).toBeGreaterThanOrEqual(0);
    const around = envExample
      .slice(Math.max(0, i - 600), i + 100)
      .toLowerCase();
    expect(around).toMatch(/server-only|server only/);
    expect(around).toMatch(/tls|https/);
    expect(around).toMatch(/plain http|never/);
  });

  it("Cookie secure: README lists the COOKIE_SECURE variable", () => {
    expect(readme).toContain("COOKIE_SECURE");
  });

  it("Cookie secure: README drops the x-forwarded-proto known gap", () => {
    const gaps = readme.slice(readme.indexOf("## 9. Known gaps"));
    expect(gaps).not.toMatch(/non-secure/i);
    expect(gaps).not.toMatch(/does not set that header/i);
  });
});
