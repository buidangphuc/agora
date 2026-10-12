// add-ga4-destination 1.2 — GET /api/analytics/identity scenarios.
// The session JWT (sub = principal id) and the agora_consent cookie are fed
// through a faked next/headers cookie jar AND the request Cookie header, so the
// real session/consent readers run. Only the HMAC key comes from env.
import { createHmac } from "node:crypto";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

let jar: Record<string, string> = {};
vi.mock("next/headers", () => ({
  cookies: () => ({
    get: (n: string) => (n in jar ? { name: n, value: jar[n] } : undefined),
  }),
  headers: () => new Headers(),
}));

import { GET } from "./route";

const KEY = "test-hmac-key";
const SUB = "buyer-42";
const b64 = (o: object) => Buffer.from(JSON.stringify(o)).toString("base64url");
const jwt = (sub: string) =>
  `${b64({ alg: "HS256", typ: "JWT" })}.${b64({
    sub,
    name: "Alice Nguyen",
    exp: Math.floor(Date.now() / 1000) + 3600,
  })}.sig`;
const ref = (id: string) => createHmac("sha256", KEY).update(id).digest("hex");
const consent = (a: 0 | 1) => `v=1&a=${a}&m=0&p=0&ts=1700000000`;

function call(
  opts: {
    a?: 0 | 1;
    session?: string | null;
    url?: string;
    init?: RequestInit;
  } = {},
) {
  jar = {};
  const parts: string[] = [];
  if (opts.a !== undefined) {
    jar.agora_consent = consent(opts.a);
    parts.push(`agora_consent=${jar.agora_consent}`);
  }
  if (opts.session) {
    jar.session = opts.session;
    parts.push(`session=${opts.session}`);
  }
  const headers = new Headers(opts.init?.headers);
  if (parts.length) headers.set("cookie", parts.join("; "));
  return GET(
    new Request(opts.url ?? "http://localhost/api/analytics/identity", {
      ...opts.init,
      headers,
    }),
  );
}

let logs: ReturnType<typeof vi.spyOn>[];
beforeEach(() => {
  process.env.GA4_USER_ID_HMAC_KEY = KEY;
  logs = (["log", "info", "warn", "error", "debug"] as const).map((m) =>
    vi.spyOn(console, m).mockImplementation(() => {}),
  );
});
afterEach(() => {
  delete process.env.GA4_USER_ID_HMAC_KEY;
  vi.restoreAllMocks();
});

const logged = () => JSON.stringify(logs.flatMap((s) => s.mock.calls));

describe("identity route", () => {
  it("[Logged-in buyer with analytics consent gets a stable HMAC] 200 twice with same hex", async () => {
    const r1 = await call({ a: 1, session: jwt(SUB) });
    const r2 = await call({ a: 1, session: jwt(SUB) });
    expect(r1.status).toBe(200);
    expect(r2.status).toBe(200);
    const b1 = await r1.text();
    const b2 = await r2.text();
    expect(b1).toBe(b2);
    const { userId } = JSON.parse(b1);
    expect(userId).toBe(ref(SUB));
    expect(userId).toMatch(/^[0-9a-f]{64}$/);
  });

  it("[Logged-in buyer ...] body has no raw user id, email or name", async () => {
    const body = await (await call({ a: 1, session: jwt(SUB) })).text();
    expect(body).not.toContain(SUB);
    expect(body).not.toContain("Alice");
    expect(body).not.toContain("@");
    expect(Object.keys(JSON.parse(body))).toEqual(["userId"]);
  });

  it("[No analytics consent yields no user_id] a=0 -> 204 empty body", async () => {
    const r = await call({ a: 0, session: jwt(SUB) });
    expect(r.status).toBe(204);
    expect(await r.text()).toBe("");
  });

  it("[No analytics consent yields no user_id] no consent cookie -> 204 empty body", async () => {
    const r = await call({ session: jwt(SUB) });
    expect(r.status).toBe(204);
    expect(await r.text()).toBe("");
  });

  it("[No analytics consent yields no user_id] malformed consent cookie -> 204", async () => {
    jar = { session: jwt(SUB), agora_consent: "garbage" };
    const r = await GET(
      new Request("http://localhost/api/analytics/identity", {
        headers: { cookie: `agora_consent=garbage; session=${jar.session}` },
      }),
    );
    expect(r.status).toBe(204);
  });

  it("[Anonymous caller yields no user_id] a=1 without session -> 204", async () => {
    const r = await call({ a: 1 });
    expect(r.status).toBe(204);
    expect(await r.text()).toBe("");
  });

  it("[Anonymous caller yields no user_id] garbage session token -> 204", async () => {
    expect((await call({ a: 1, session: "not-a-jwt" })).status).toBe(204);
  });

  it("[Missing key disables the feature] unset key -> 204 for a consented, logged-in caller", async () => {
    delete process.env.GA4_USER_ID_HMAC_KEY;
    const r = await call({ a: 1, session: jwt(SUB) });
    expect(r.status).toBe(204);
    expect(await r.text()).toBe("");
  });

  it("[Missing key disables the feature] empty key -> 204 and nothing logged with key or user id", async () => {
    process.env.GA4_USER_ID_HMAC_KEY = "";
    const r = await call({ a: 1, session: jwt(SUB) });
    expect(r.status).toBe(204);
    expect(logged()).not.toContain(SUB);
  });

  it("[Missing key disables the feature] no key or user id appears in logs on any path", async () => {
    await call({ a: 1, session: jwt(SUB) });
    await call({ a: 0, session: jwt(SUB) });
    await call({ a: 1 });
    const out = logged();
    expect(out).not.toContain(KEY);
    expect(out).not.toContain(SUB);
    expect(out).not.toContain(ref(SUB));
  });

  it("[Caller cannot request another user's id] userId query param is ignored", async () => {
    const r = await call({
      a: 1,
      session: jwt(SUB),
      url: "http://localhost/api/analytics/identity?userId=victim-1&sub=victim-1",
    });
    expect(JSON.parse(await r.text()).userId).toBe(ref(SUB));
  });

  it("[Caller cannot request another user's id] userId in body is ignored", async () => {
    jar = {};
    const r = await call({
      a: 1,
      session: jwt(SUB),
      init: { headers: { "content-type": "application/json" } },
    });
    expect(JSON.parse(await r.text()).userId).toBe(ref(SUB));
  });

  it("[Caller cannot request another user's id] anonymous caller naming a user still gets 204", async () => {
    const r = await call({
      a: 1,
      url: "http://localhost/api/analytics/identity?userId=victim-1",
    });
    expect(r.status).toBe(204);
  });

  it("[Response is not cacheable] 200 has Cache-Control no-store", async () => {
    const r = await call({ a: 1, session: jwt(SUB) });
    expect(r.status).toBe(200);
    expect(r.headers.get("cache-control")).toContain("no-store");
  });

  it("[Response is not cacheable] 204 has Cache-Control no-store", async () => {
    const r = await call({ a: 1 });
    expect(r.status).toBe(204);
    expect(r.headers.get("cache-control")).toContain("no-store");
  });

  it("[Consent withdrawal stops user_id] same session: a=1 -> 200, then a=0 -> 204", async () => {
    const session = jwt(SUB);
    expect((await call({ a: 1, session })).status).toBe(200);
    expect((await call({ a: 0, session })).status).toBe(204);
  });
});
