// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";

const GOOGLE_HOSTS = [
  "https://www.googletagmanager.com",
  "https://*.google-analytics.com",
  "https://*.analytics.google.com",
  "https://region1.google-analytics.com",
];
const GOOGLE_DIRECTIVES = ["script-src", "connect-src", "img-src"];

async function cspFor(gtmId: string | undefined): Promise<string> {
  vi.resetModules();
  vi.stubEnv("NEXT_PUBLIC_GTM_ID", gtmId ?? "");
  const mod = await import("../next.config.mjs");
  const cfg = mod.default as {
    headers?: () => Promise<
      { source: string; headers: { key: string; value: string }[] }[]
    >;
  };
  expect(typeof cfg.headers).toBe("function");
  const rules = await cfg.headers!();
  const csp = rules
    .flatMap((r) => r.headers)
    .find((h) => h.key.toLowerCase() === "content-security-policy");
  expect(csp, "Content-Security-Policy header present").toBeDefined();
  return csp!.value;
}

function directive(csp: string, name: string): string[] {
  const d = csp
    .split(";")
    .map((s) => s.trim())
    .find((s) => s.startsWith(`${name} `));
  expect(d, `${name} directive present`).toBeDefined();
  return d!.split(/\s+/).slice(1);
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
});
