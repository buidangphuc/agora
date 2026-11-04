// @vitest-environment node
import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const refreshSession = vi.hoisted(() => vi.fn());
vi.mock("@/lib/gateway/refresh", () => ({ refreshSession }));

import { GET } from "./app/auth/handoff/route";
import { middleware } from "./middleware";

const CODE = "hc_9f3a7c1e5b2d4a60";
const BAD = "hc_BADBADBAD0000";

let lines: string[] = [];

function render(args: unknown[]): string {
  return args
    .map((a) => (typeof a === "string" ? a : JSON.stringify(a)))
    .join(" ");
}

beforeEach(() => {
  lines = [];
  for (const m of ["log", "info", "warn", "error", "debug"] as const) {
    vi.spyOn(console, m).mockImplementation((...a: unknown[]) => {
      lines.push(render(a));
    });
  }
  vi.spyOn(process.stdout, "write").mockImplementation(((c: unknown) => {
    lines.push(String(c));
    return true;
  }) as typeof process.stdout.write);
  vi.spyOn(process.stderr, "write").mockImplementation(((c: unknown) => {
    lines.push(String(c));
    return true;
  }) as typeof process.stderr.write);
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  refreshSession.mockReset();
});

function handoffLines(): string[] {
  return lines.filter((l) => l.includes("/auth/handoff"));
}

describe("access log redacts handoff code", () => {
  it("[Code redacted in access logs] the request line shows code=REDACTED and never the code", async () => {
    await middleware(
      new NextRequest(`http://app.test/auth/handoff?code=${CODE}&next=/x`),
    );
    const hit = handoffLines();
    expect(hit.length).toBeGreaterThan(0);
    expect(hit.some((l) => l.includes("code=REDACTED"))).toBe(true);
    expect(lines.join("\n")).not.toContain(CODE);
  });

  it("[Rejected hand-off request is still redacted] a failing redemption logs nothing with the code", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("{}", { status: 404 })),
    );
    const url = `http://app.test/auth/handoff?code=${BAD}`;
    await middleware(new NextRequest(url));
    const res = await GET(new Request(url));
    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toMatch(/^\/login/);
    expect(handoffLines().some((l) => l.includes("code=REDACTED"))).toBe(true);
    expect(lines.join("\n")).not.toContain(BAD);
  });

  it("[Anonymous request is redacted without a session] no session or refresh cookie still logs redacted", async () => {
    await middleware(
      new NextRequest(`http://app.test/auth/handoff?code=${CODE}`),
    );
    expect(handoffLines().some((l) => l.includes("code=REDACTED"))).toBe(true);
    expect(lines.join("\n")).not.toContain(CODE);
  });

  it("[Other query parameters keep their value] next=/x stays readable in the line", async () => {
    await middleware(
      new NextRequest(
        `http://app.test/auth/handoff?code=${CODE}&next=/x&utm=a`,
      ),
    );
    const line = handoffLines().find((l) => l.includes("code=REDACTED"));
    expect(line).toBeDefined();
    expect(line).toContain("next=/x");
    expect(line).toContain("utm=a");
    expect(lines.join("\n")).not.toContain(CODE);
  });

  it("[Other query parameters keep their value] other paths are logged untouched", async () => {
    await middleware(
      new NextRequest("http://app.test/search?code=PROMO10&q=shoes"),
    );
    const line = lines.find((l) => l.includes("/search"));
    expect(line).toBeDefined();
    expect(line).toContain("code=PROMO10");
    expect(line).toContain("q=shoes");
  });
});

describe("access log redacts handoff code in alternate encodings", () => {
  const SECRET = "SECRETVALUE123";

  async function logFor(query: string, path = "/auth/handoff") {
    await middleware(new NextRequest(`http://app.test${path}?${query}`));
    return handoffLines();
  }

  it.each([
    ["lowercase hex", "c%6fde=SECRETVALUE123"],
    ["uppercase hex", "c%6Fde=SECRETVALUE123"],
    ["fully encoded", "%63%6F%64%65=SECRETVALUE123"],
  ])(
    "[Code redacted in access logs] percent-encoded parameter name (%s) is redacted",
    async (_n, q) => {
      const hit = await logFor(`${q}&next=/x`);
      expect(hit.length).toBeGreaterThan(0);
      expect(hit.some((l) => l.includes("REDACTED"))).toBe(true);
      expect(lines.join("\n")).not.toContain(SECRET);
      expect(hit.some((l) => l.includes("next=/x"))).toBe(true);
    },
  );

  it("[Code redacted in access logs] a repeated code parameter has every value redacted", async () => {
    const hit = await logFor("code=SECRETONE&next=/x&code=SECRETTWO");
    expect(hit.some((l) => l.includes("code=REDACTED"))).toBe(true);
    const all = lines.join("\n");
    expect(all).not.toContain("SECRETONE");
    expect(all).not.toContain("SECRETTWO");
  });

  it("[Code redacted in access logs] a trailing-slash path is redacted", async () => {
    const hit = await logFor(`code=${SECRET}&next=/x`, "/auth/handoff/");
    expect(hit.some((l) => l.includes("code=REDACTED"))).toBe(true);
    expect(lines.join("\n")).not.toContain(SECRET);
  });

  it("[Rejected hand-off request is still redacted] an undecodable key redacts the whole query", async () => {
    const hit = await logFor(`%E0%A4%A=1&code=${SECRET}&next=/x`);
    expect(hit.length).toBeGreaterThan(0);
    expect(hit.some((l) => l.includes("REDACTED"))).toBe(true);
    expect(lines.join("\n")).not.toContain(SECRET);
  });

  it("[Other query parameters keep their value] a lookalike encoded name is not masked", async () => {
    const hit = await logFor("xc%6fde=keep&next=/x");
    expect(hit.some((l) => l.includes("keep"))).toBe(true);
  });
});
