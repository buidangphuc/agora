// @vitest-environment node
import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const refreshSession = vi.hoisted(() => vi.fn());
vi.mock("@/lib/gateway/refresh", () => ({ refreshSession }));

import { middleware } from "./middleware";
import { GET } from "./app/auth/handoff/route";

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
      new NextRequest(
        `http://app.test/auth/handoff?code=${CODE}&next=/x`,
      ),
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
