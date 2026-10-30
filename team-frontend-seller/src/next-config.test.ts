// @vitest-environment node
import { readFileSync } from "node:fs";
import { afterEach, describe, expect, it, vi } from "vitest";

const GOOGLE_HOSTS = [
  "https://www.googletagmanager.com",
  "https://*.google-analytics.com",
  "https://*.analytics.google.com",
  "https://region1.google-analytics.com",
];
const GOOGLE_DIRECTIVES = ["script-src", "connect-src", "img-src"];

async function cspFor(
  gtmId: string | undefined,
  env: Record<string, string> = {},
): Promise<string> {
  vi.resetModules();
  vi.stubEnv("NEXT_PUBLIC_GTM_ID", gtmId ?? "");
  for (const [k, v] of Object.entries(env)) vi.stubEnv(k, v);
  const mod = await import("../next.config.mjs");
  const cfg = mod.default as {
    headers?: () => Promise<
      { source: string; headers: { key: string; value: string }[] }[]
    >;
  };
  expect(typeof cfg.headers).toBe("function");
  const rules = (await cfg.headers?.()) ?? [];
  const csp = rules
    .flatMap((r) => r.headers)
    .find((h) => h.key.toLowerCase() === "content-security-policy");
  expect(csp, "Content-Security-Policy header present").toBeDefined();
  return csp?.value ?? "";
}

function directive(csp: string, name: string): string[] {
  const d = csp
    .split(";")
    .map((s) => s.trim())
    .find((s) => s.startsWith(`${name} `));
  expect(d, `${name} directive present`).toBeDefined();
  return (d ?? "").split(/\s+/).slice(1);
}

afterEach(() => vi.unstubAllEnvs());

describe("CSP (add-ga4-destination)", () => {
  it("Configured environment allowlists the Google hosts", async () => {
    const csp = await cspFor("GTM-XXXX");
    for (const name of GOOGLE_DIRECTIVES) {
      const sources = directive(csp, name);
      for (const host of GOOGLE_HOSTS) expect(sources).toContain(host);
    }
  });

  it("Unconfigured environment has no Google host", async () => {
    for (const id of [undefined, "   "]) {
      const csp = await cspFor(id);
      expect(csp).not.toMatch(/google/i);
      for (const name of GOOGLE_DIRECTIVES) directive(csp, name);
    }
  });

  it("Unlisted host is blocked: no wildcard source and no arbitrary third party", async () => {
    for (const id of [undefined, "GTM-XXXX"]) {
      const csp = await cspFor(id);
      expect(csp).not.toContain("evil.example");
      for (const name of GOOGLE_DIRECTIVES) {
        const sources = directive(csp, name);
        expect(sources).not.toContain("*");
        expect(sources).not.toContain("https:");
        expect(sources).not.toContain("http:");
      }
      expect(csp).not.toMatch(/facebook|doubleclick|hotjar|mixpanel/i);
    }
  });

  it("Existing app keeps working: self and Next bootstrap inline scripts stay allowed", async () => {
    const csp = await cspFor(undefined);
    expect(directive(csp, "script-src")).toContain("'self'");
    expect(directive(csp, "script-src")).toContain("'unsafe-inline'");
    expect(directive(csp, "connect-src")).toContain("'self'");
    expect(directive(csp, "img-src")).toContain("'self'");
    expect(directive(csp, "img-src")).toContain("data:");
  });

  it("Dev server works: non-production allows unsafe-eval and ws: for HMR", async () => {
    const csp = await cspFor(undefined, { NODE_ENV: "development" });
    expect(directive(csp, "script-src")).toContain("'unsafe-eval'");
    expect(directive(csp, "connect-src").some((s) => s.startsWith("ws:"))).toBe(
      true,
    );
  });

  it("Production stays strict: no unsafe-eval and no ws: sources", async () => {
    const csp = await cspFor("GTM-XXXX", { NODE_ENV: "production" });
    expect(csp).not.toContain("'unsafe-eval'");
    expect(csp).not.toMatch(/\bwss?:/);
  });

  it("Gateway and media origins are allowlisted when set", async () => {
    const csp = await cspFor(undefined, {
      NODE_ENV: "production",
      NEXT_PUBLIC_GATEWAY_URL: "https://api.example.test/v1",
      NEXT_PUBLIC_MEDIA_BASE_URL: "https://media.example.test/listing-images",
    });
    expect(directive(csp, "connect-src")).toContain("https://api.example.test");
    expect(directive(csp, "img-src")).toContain("https://media.example.test");
    expect(csp).not.toContain("listing-images");
  });

  it("Upload previews work: img-src allows blob:", async () => {
    const csp = await cspFor(undefined, { NODE_ENV: "production" });
    expect(directive(csp, "img-src")).toContain("blob:");
  });

  it("Build-time CSP inputs: Dockerfile declares ARG and ENV for gateway and media URLs", () => {
    const df = readFileSync(new URL("../Dockerfile", import.meta.url), "utf8");
    for (const v of ["NEXT_PUBLIC_GATEWAY_URL", "NEXT_PUBLIC_MEDIA_BASE_URL"]) {
      expect(df).toMatch(new RegExp(`^ARG ${v}(=|\\s*$)`, "m"));
      expect(df).toMatch(new RegExp(`^ENV ${v}=`, "m"));
    }
  });
});
