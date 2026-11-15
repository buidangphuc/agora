import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

let jar: Record<string, string> = {};
vi.mock("next/headers", () => ({
  cookies: () => ({
    get: (n: string) => (n in jar ? { name: n, value: jar[n] } : undefined),
  }),
  headers: () => new Headers(),
}));

const recordAdClick = vi.hoisted(() => vi.fn());
vi.mock("@/lib/gateway/promotion", () => ({ recordAdClick }));

import { getToken } from "@/lib/gateway/session";

import { GET, POST } from "./route";

const b64 = (o: object) => Buffer.from(JSON.stringify(o)).toString("base64url");
const SESSION = `${b64({ alg: "HS256", typ: "JWT" })}.${b64({
  sub: "buyer-user-42",
  exp: Math.floor(Date.now() / 1000) + 3600,
})}.sig`;

function post(body: BodyInit | null, withSession = false) {
  jar = withSession ? { session: SESSION } : {};
  return POST(
    new Request("http://localhost/api/ads/click", {
      method: "POST",
      headers: {
        "content-type": "application/json",
        ...(withSession ? { cookie: `session=${SESSION}` } : {}),
      },
      body,
    }),
  );
}

let logged: string[];
function captureLogs() {
  logged = [];
  for (const m of ["log", "info", "warn", "error", "debug"] as const) {
    vi.spyOn(console, m).mockImplementation((...a: unknown[]) => {
      logged.push(a.map((x) => (x instanceof Error ? x.message : String(x))).join(" "));
    });
  }
}

beforeEach(() => {
  vi.clearAllMocks();
  jar = {};
  recordAdClick.mockResolvedValue({ listingId: "listing-77", charged: true });
  captureLogs();
});
afterEach(() => vi.restoreAllMocks());

describe("POST /api/ads/click", () => {
  it("rejects malformed bodies with 400 (Click route rejects a malformed body)", async () => {
    const bad: Array<BodyInit | null> = [
      "",
      null,
      "not json",
      JSON.stringify({}),
      JSON.stringify({ token: 5 }),
      JSON.stringify({ token: "" }),
      JSON.stringify({ token: "a".repeat(257) }),
      JSON.stringify({ token: "has space" }),
      JSON.stringify({ token: "bad/char" }),
      JSON.stringify({ token: "bad%char" }),
    ];
    for (const body of bad) {
      const res = await post(body);
      expect(res.status, String(body)).toBe(400);
    }
    expect(recordAdClick).not.toHaveBeenCalled();
  });

  it("accepts boundary tokens of 1 and 256 allowed characters (Click route rejects a malformed body)", async () => {
    for (const token of ["a", "A-z_0.9~".padEnd(256, "x")]) {
      const res = await post(JSON.stringify({ token }));
      expect(res.status).toBe(204);
      expect(recordAdClick).toHaveBeenLastCalledWith(token);
    }
  });

  it("answers 204 for charged, uncharged and failed calls (Click route hides the billing outcome)", async () => {
    const outcomes = [
      () => recordAdClick.mockResolvedValue({ listingId: "l", charged: true }),
      () => recordAdClick.mockResolvedValue({ listingId: "l", charged: false }),
      () => recordAdClick.mockRejectedValue(new Error("upstream down")),
    ];
    for (const arrange of outcomes) {
      arrange();
      const res = await post(JSON.stringify({ token: "tok-1" }));
      expect(res.status).toBe(204);
      expect(await res.text()).toBe("");
    }
    expect(recordAdClick).toHaveBeenCalledTimes(3);
  });

  it("forwards session and logs outcome class only (Click route forwards the session and does not log ids)", async () => {
    let seenBearer: string | undefined;
    recordAdClick.mockImplementation(async () => {
      seenBearer = getToken();
      return { listingId: "listing-77", charged: true };
    });

    const res = await post(JSON.stringify({ token: "secret-token-1" }), true);
    expect(res.status).toBe(204);
    expect(seenBearer).toBe(SESSION);

    recordAdClick.mockRejectedValue(
      new Error("failed secret-token-1 listing-77 buyer-user-42"),
    );
    await post(JSON.stringify({ token: "secret-token-1" }), true);

    const all = logged.join("\n");
    expect(all).not.toContain("secret-token-1");
    expect(all).not.toContain("listing-77");
    expect(all).not.toContain("buyer-user-42");
    expect(all).not.toContain(SESSION);
  });

  it("GET is rejected with 405 (GET on the click route is not allowed)", async () => {
    expect(typeof GET).toBe("function");
    const res = await GET(
      new Request("http://localhost/api/ads/click?token=x", { method: "GET" }),
    );
    expect(res.status).toBe(405);
    expect(recordAdClick).not.toHaveBeenCalled();
  });
});
