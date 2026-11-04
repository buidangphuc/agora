import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { jwt, nowSec } from "./__helpers__/identity";
import { FakeJar } from "./__helpers__/jar";
import { SESSION_PRESETS } from "./config";

const jar = new FakeJar();

vi.mock("next/headers", () => ({
  cookies: async () => jar,
  headers: async () => jar.headers(),
}));
vi.mock("next/navigation", () => ({
  notFound: () => {
    throw new Error("NEXT_NOT_FOUND");
  },
  redirect: (to: string) => {
    throw new Error(`NEXT_REDIRECT:${to}`);
  },
}));

import { createHandoffHandler, startHandoff } from "./handoff";

const ORIGIN = "https://seller.example";
const CODE = "CODE-ABC-123-SECRET";
const TTL_MS = 60_000;

class Unauthenticated extends Error {
  readonly code = 16;
}
class Unavailable extends Error {
  readonly code = 14;
}

class FakeGateway {
  now = 0;
  peekCalls = 0;
  redeemCalls = 0;
  outage = false;
  private readonly codes = new Map<
    string,
    { username: string; mintedAt: number; used: boolean; sub: string }
  >();

  mint(code: string, username: string, sub = `sub-${username}`) {
    this.codes.set(code, {
      username,
      mintedAt: this.now,
      used: false,
      sub,
    });
  }

  private live(code: string) {
    const entry = this.codes.get(code);
    if (!entry || entry.used || this.now - entry.mintedAt > TTL_MS) {
      throw new Unauthenticated("invalid code");
    }
    return entry;
  }

  peek = async (code: string) => {
    this.peekCalls++;
    if (this.outage) throw new Unavailable("down");
    return { username: this.live(code).username };
  };

  redeem = async (code: string) => {
    this.redeemCalls++;
    if (this.outage) throw new Unavailable("down");
    const entry = this.live(code);
    entry.used = true;
    return {
      token: jwt({
        exp: nowSec() + 3600,
        sub: entry.sub,
        name: entry.username,
      }),
      refreshToken: `refresh-${entry.username}`,
    };
  };

  isRedeemable(code: string): boolean {
    try {
      this.live(code);
      return true;
    } catch {
      return false;
    }
  }
}

let gateway: FakeGateway;

function seller(origin = ORIGIN) {
  return createHandoffHandler(SESSION_PRESETS.seller, {
    origin,
    peek: (c: string) => gateway.peek(c),
    redeem: (c: string) => gateway.redeem(c),
  });
}

function getReq(query: string, headers: Record<string, string> = {}) {
  return new NextRequest(`${ORIGIN}/auth/handoff${query}`, { headers });
}

function postReq(
  fields: Record<string, string>,
  headers: Record<string, string> = {},
  url = `${ORIGIN}/auth/handoff`,
) {
  return new NextRequest(url, {
    method: "POST",
    headers,
    body: new URLSearchParams(fields),
  });
}

const sameOrigin = { origin: ORIGIN };

function location(res: Response): URL {
  const loc = res.headers.get("location");
  expect(loc, "location header").toBeTruthy();
  return new URL(loc as string, ORIGIN);
}

function expectLogin(res: Response, next?: string) {
  expect(res.status).toBe(303);
  const to = location(res);
  expect(to.origin).toBe(ORIGIN);
  expect(to.pathname).toBe("/login");
  if (next === undefined) {
    expect(to.search).toBe("");
  } else {
    expect(to.searchParams.get("next")).toBe(next);
  }
  expect(res.headers.get("referrer-policy")).toBe("same-origin");
}

function sessionCookies() {
  return [...jar.store.keys()].filter((k) => /session|refresh/.test(k));
}

function consoleOutput(spies: ReturnType<typeof vi.spyOn>[]): string {
  return spies
    .flatMap((s) => s.mock.calls)
    .map((args) => args.map((a: unknown) => String(a)).join(" "))
    .join("\n");
}

beforeEach(() => {
  jar.store.clear();
  jar.deleted.length = 0;
  gateway = new FakeGateway();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("GET /auth/handoff", () => {
  it("GET shows the confirmation page and consumes nothing", async () => {
    gateway.mint(CODE, "alice");
    const res = await seller().GET(getReq(`?code=${CODE}&next=/seller/orders`));
    expect(res.status).toBe(200);
    const html = await res.text();
    expect(html).toContain("Tiếp tục với tài khoản alice?");
    expect(html).toMatch(/<form[^>]*method=["']post["']/i);
    expect(res.headers.get("set-cookie")).toBeNull();
    expect(sessionCookies()).toEqual([]);
    expect(gateway.redeemCalls).toBe(0);
    expect(gateway.isRedeemable(CODE)).toBe(true);
    expect(res.headers.get("referrer-policy")).toBe("same-origin");
  });

  it("HEAD and prefetch never consume", async () => {
    gateway.mint(CODE, "alice");
    const handler = seller();
    const head = await handler.HEAD(
      new NextRequest(`${ORIGIN}/auth/handoff?code=${CODE}`, {
        method: "HEAD",
      }),
    );
    expect(head.headers.get("referrer-policy")).toBe("same-origin");
    const prefetch = await handler.GET(
      getReq(`?code=${CODE}`, { purpose: "prefetch" }),
    );
    expect(prefetch.headers.get("referrer-policy")).toBe("same-origin");
    const secPrefetch = await handler.GET(
      getReq(`?code=${CODE}`, { "sec-purpose": "prefetch" }),
    );
    expect(secPrefetch.headers.get("referrer-policy")).toBe("same-origin");
    expect(gateway.redeemCalls).toBe(0);
    expect(sessionCookies()).toEqual([]);
    expect(gateway.isRedeemable(CODE)).toBe(true);
  });

  it("Confirmation page cannot be framed or cached", async () => {
    gateway.mint(CODE, "alice");
    const res = await seller().GET(getReq(`?code=${CODE}&next=/seller`));
    expect(res.status).toBe(200);
    expect(res.headers.get("content-security-policy")).toContain(
      "frame-ancestors 'none'",
    );
    expect(res.headers.get("x-frame-options")).toBe("DENY");
    expect(res.headers.get("cache-control")).toContain("no-store");
    expect(res.headers.get("referrer-policy")).toBe("same-origin");
  });

  it("Code is in the POST body only", async () => {
    gateway.mint(CODE, "alice");
    const res = await seller().GET(getReq(`?code=${CODE}&next=/seller/x`));
    const html = await res.text();
    const form = html.match(/<form\b[^>]*>/i)?.[0] ?? "";
    expect(form).toMatch(/action=["']\/auth\/handoff["']/);
    expect(form).not.toContain("?");
    expect(form).not.toContain(CODE);
    expect(html).toMatch(
      new RegExp(
        `<input[^>]*type=["']hidden["'][^>]*name=["']code["'][^>]*value=["']${CODE}["']|<input[^>]*name=["']code["'][^>]*type=["']hidden["'][^>]*value=["']${CODE}["']`,
        "i",
      ),
    );
    expect(html.split(CODE).length - 1).toBe(1);
    expect(html).not.toMatch(/<script\b/i);
    expect(html).not.toMatch(/<link\b/i);
    expect(html).not.toMatch(/<img\b/i);
    expect(html).not.toMatch(/(src|href)\s*=\s*["']?(https?:)?\/\//i);
    expect(html).not.toMatch(/url\(\s*["']?(https?:)?\/\//i);
    expect(html).not.toMatch(/@import/i);
  });

  it("Username is escaped and truncated", async () => {
    const handler = seller();
    gateway.mint("C1", "<script>alert(1)</script>");
    const xss = await (await handler.GET(getReq("?code=C1"))).text();
    expect(xss).toContain("&lt;script&gt;");
    expect(xss).not.toMatch(/<script/i);

    gateway.mint("C2", "x".repeat(500));
    const long = await (await handler.GET(getReq("?code=C2"))).text();
    expect(long).toMatch(/x{10}/);
    expect(long).not.toMatch(/x{65}/);
  });

  it("invalid code on GET falls back to login", async () => {
    const handler = seller();
    gateway.mint("USED", "alice");
    await gateway.redeem("USED");
    for (const code of ["USED", "UNKNOWN"]) {
      const res = await handler.GET(
        getReq(`?code=${code}&next=/seller/orders`),
      );
      expectLogin(res, "/seller/orders");
      expect(res.headers.get("set-cookie")).toBeNull();
    }
    expect(sessionCookies()).toEqual([]);
  });

  it("rejects missing or oversized code", async () => {
    const handler = seller();
    const missing = await handler.GET(getReq("?next=/seller"));
    expect(missing.status).toBe(303);
    expect(location(missing).pathname).toBe("/login");
    const oversized = await handler.GET(getReq(`?code=${"a".repeat(65)}`));
    expect(oversized.status).toBe(303);
    expect(location(oversized).pathname).toBe("/login");
    const missingPost = await handler.POST(postReq({}, sameOrigin));
    expect(missingPost.status).toBe(303);
    expect(location(missingPost).pathname).toBe("/login");
    const oversizedPost = await handler.POST(
      postReq({ code: "a".repeat(65) }, sameOrigin),
    );
    expect(oversizedPost.status).toBe(303);
    expect(location(oversizedPost).pathname).toBe("/login");
    expect(gateway.peekCalls).toBe(0);
    expect(gateway.redeemCalls).toBe(0);
  });

  it("neutralises unsafe next", async () => {
    const handler = seller();
    for (const evil of ["https://evil.example/x", "//evil.example"]) {
      gateway.mint("BAD", "alice");
      const res = await handler.GET(
        getReq(`?code=nope&next=${encodeURIComponent(evil)}`),
      );
      expect(location(res).origin).toBe(ORIGIN);
      expect(location(res).searchParams.get("next")).toBe("/");

      const ok = await handler.POST(
        postReq({ code: "BAD", next: evil }, sameOrigin),
      );
      expect(ok.status).toBe(303);
      const to = location(ok);
      expect(to.origin).toBe(ORIGIN);
      expect(to.pathname).toBe("/");
      expect(ok.headers.get("location")).not.toContain("evil");
    }
  });

  it("gateway outage falls back to login", async () => {
    const handler = seller();
    gateway.mint(CODE, "alice");
    gateway.outage = true;
    const get = await handler.GET(getReq(`?code=${CODE}&next=/seller/orders`));
    expectLogin(get, "/seller/orders");
    const post = await handler.POST(
      postReq({ code: CODE, next: "/seller/orders" }, sameOrigin),
    );
    expectLogin(post, "/seller/orders");
    expect(sessionCookies()).toEqual([]);
  });
});

describe("POST /auth/handoff", () => {
  it("POST redeems and redirects without the code", async () => {
    gateway.mint(CODE, "alice");
    const res = await seller().POST(
      postReq({ code: CODE, next: "/seller/orders" }, sameOrigin),
    );
    expect(res.status).toBe(303);
    const to = location(res);
    expect(to.pathname).toBe("/seller/orders");
    expect(to.search).toBe("");
    expect(res.headers.get("location")).not.toContain("code");
    expect(res.headers.get("location")).not.toContain(CODE);
    expect(res.headers.get("referrer-policy")).toBe("same-origin");
    expect(jar.store.has("seller_session")).toBe(true);
    expect(jar.store.has("seller_refresh")).toBe(true);
    expect(gateway.redeemCalls).toBe(1);
  });

  it("Forged cross-site POST is refused", async () => {
    gateway.mint(CODE, "alice");
    const handler = seller();
    for (const headers of [
      { origin: "https://evil.example" },
      { "sec-fetch-site": "cross-site" },
      { "sec-fetch-site": "same-site" },
    ] as Record<string, string>[]) {
      const res = await handler.POST(
        postReq({ code: CODE, next: "/seller/orders" }, headers),
      );
      expectLogin(res);
      expect(res.headers.get("set-cookie")).toBeNull();
    }
    expect(gateway.redeemCalls).toBe(0);
    expect(sessionCookies()).toEqual([]);
    expect(gateway.isRedeemable(CODE)).toBe(true);
  });

  it("Origin matching Host but not the configured origin is refused", async () => {
    gateway.mint(CODE, "alice");
    const res = await seller().POST(
      postReq(
        { code: CODE, next: "/seller/orders" },
        {
          origin: "https://seller.attacker.example",
          host: "seller.attacker.example",
        },
        "https://seller.attacker.example/auth/handoff",
      ),
    );
    expectLogin(res);
    expect(gateway.redeemCalls).toBe(0);
    expect(sessionCookies()).toEqual([]);
  });

  it("Evil Origin with same-origin Sec-Fetch-Site is refused", async () => {
    gateway.mint(CODE, "alice");
    const res = await seller().POST(
      postReq(
        { code: CODE },
        { origin: "https://evil.example", "sec-fetch-site": "same-origin" },
      ),
    );
    expectLogin(res);
    expect(gateway.redeemCalls).toBe(0);
    expect(sessionCookies()).toEqual([]);
  });

  it("Origin null is refused", async () => {
    gateway.mint(CODE, "alice");
    const res = await seller().POST(
      postReq(
        { code: CODE },
        { origin: "null", "sec-fetch-site": "same-origin" },
      ),
    );
    expectLogin(res);
    expect(gateway.redeemCalls).toBe(0);
    expect(sessionCookies()).toEqual([]);
  });

  it("Refused POST sends no-referrer and no next", async () => {
    gateway.mint(CODE, "alice");
    const res = await seller().POST(
      postReq(
        { code: CODE, next: "/seller/orders" },
        { origin: "https://evil.example" },
      ),
    );
    expect(res.status).toBe(303);
    expect(location(res).pathname).toBe("/login");
    expect(location(res).search).toBe("");
    expect(res.headers.get("location")).not.toContain("next");
    expect(res.headers.get("referrer-policy")).toBe("same-origin");
  });

  it("POST with neither Origin nor Sec-Fetch-Site is refused", async () => {
    gateway.mint(CODE, "alice");
    const res = await seller().POST(postReq({ code: CODE }));
    expectLogin(res);
    expect(gateway.redeemCalls).toBe(0);
    expect(sessionCookies()).toEqual([]);
  });

  it("Same-origin POST via Sec-Fetch-Site is accepted", async () => {
    gateway.mint(CODE, "alice");
    const res = await seller().POST(
      postReq(
        { code: CODE, next: "/seller/orders" },
        { "sec-fetch-site": "same-origin" },
      ),
    );
    expect(gateway.redeemCalls).toBe(1);
    expect(res.status).toBe(303);
    expect(location(res).pathname).toBe("/seller/orders");
    expect(jar.store.has("seller_session")).toBe(true);
  });

  it("Dwelling past the TTL falls back to login", async () => {
    gateway.mint(CODE, "alice");
    const handler = seller();
    const page = await handler.GET(getReq(`?code=${CODE}&next=/seller/orders`));
    expect(page.status).toBe(200);
    gateway.now = 61_000;
    const res = await handler.POST(
      postReq({ code: CODE, next: "/seller/orders" }, sameOrigin),
    );
    expectLogin(res, "/seller/orders");
    expect(sessionCookies()).toEqual([]);
  });

  it("Failed redemption on POST falls back to login", async () => {
    const res = await seller().POST(
      postReq({ code: "UNKNOWN", next: "/seller/orders" }, sameOrigin),
    );
    expectLogin(res, "/seller/orders");
    expect(res.headers.get("set-cookie")).toBeNull();
    expect(sessionCookies()).toEqual([]);
  });
});

describe("hand-off between accounts", () => {
  it("Already signed-in user gets the confirmation and the POST replaces the session", async () => {
    const bob = jwt({ exp: nowSec() + 3600, sub: "sub-bob", name: "bob" });
    jar.store.set("seller_session", bob);
    jar.store.set("seller_refresh", "refresh-bob");
    gateway.mint(CODE, "alice");
    const handler = seller();

    const page = await handler.GET(getReq(`?code=${CODE}&next=/seller`));
    expect(page.status).toBe(200);
    expect(await page.text()).toContain("Tiếp tục với tài khoản alice?");
    expect(jar.store.get("seller_session")).toBe(bob);

    const res = await handler.POST(
      postReq({ code: CODE, next: "/seller" }, sameOrigin),
    );
    expect(res.status).toBe(303);
    expect(jar.store.get("seller_session")).not.toBe(bob);
    expect(jar.store.get("seller_refresh")).toBe("refresh-alice");
    const payload = JSON.parse(
      atob(
        (jar.store.get("seller_session") as string)
          .split(".")[1]
          .replace(/-/g, "+")
          .replace(/_/g, "/"),
      ),
    );
    expect(payload.name).toBe("alice");
  });

  it("Attacker link needs the victim's click", async () => {
    gateway.mint(CODE, "mallory");
    const handler = seller();
    const page = await handler.GET(getReq(`?code=${CODE}&next=/seller`));
    expect(await page.text()).toContain("mallory");
    expect(sessionCookies()).toEqual([]);
    expect(gateway.redeemCalls).toBe(0);

    await handler.POST(postReq({ code: CODE, next: "/seller" }, sameOrigin));
    expect(gateway.redeemCalls).toBe(1);
  });
});

describe("startHandoff", () => {
  const targets = {
    buyer: "https://buyer.example",
    seller: "https://seller.example",
  };

  function starter() {
    const mint = vi.fn(async (_token: string, _target: "buyer" | "seller") => ({
      code: "MINTED-1",
    }));
    return {
      mint,
      run: (next: string) =>
        startHandoff(SESSION_PRESETS.buyer, "seller", next, {
          mint,
          origins: targets,
        }),
    };
  }

  it("code minted on click never at render", async () => {
    const token = jwt({ exp: nowSec() + 3600, sub: "u1", name: "alice" });
    jar.store.set("session", token);
    const s = starter();
    expect(s.mint).not.toHaveBeenCalled();
    const url = new URL(await s.run("/seller/orders"));
    expect(s.mint).toHaveBeenCalledTimes(1);
    expect(s.mint).toHaveBeenCalledWith(token, "seller");
    expect(url.origin).toBe(targets.seller);
    expect(url.pathname).toBe("/auth/handoff");
    expect(url.searchParams.get("code")).toBe("MINTED-1");
    expect(url.searchParams.get("next")).toBe("/seller/orders");
  });

  it("anonymous startHandoff returns login url", async () => {
    const s = starter();
    const url = new URL(await s.run("/seller/orders"));
    expect(url.origin).toBe(targets.seller);
    expect(url.pathname).toBe("/login");
    expect(url.searchParams.get("next")).toBe("/seller/orders");
    expect(url.searchParams.get("code")).toBeNull();
    expect(s.mint).not.toHaveBeenCalled();
  });

  it("mint failure falls back to the plain login url", async () => {
    jar.store.set(
      "session",
      jwt({ exp: nowSec() + 3600, sub: "u1", name: "alice" }),
    );
    const mint = vi.fn(async () => {
      throw new Error("RESOURCE_EXHAUSTED");
    });
    const url = new URL(
      await startHandoff(SESSION_PRESETS.buyer, "seller", "/seller", {
        mint,
        origins: targets,
      }),
    );
    expect(url.origin).toBe(targets.seller);
    expect(url.pathname).toBe("/login");
    expect(url.searchParams.get("next")).toBe("/seller");
    expect(url.searchParams.get("code")).toBeNull();
  });

  it("admin preset cannot hand off", async () => {
    const mint = vi.fn(async () => ({ code: "X" }));
    await expect(
      startHandoff(SESSION_PRESETS.admin, "seller", "/seller", {
        mint,
        origins: targets,
      }),
    ).rejects.toThrow();
    expect(mint).not.toHaveBeenCalled();
    expect(() =>
      createHandoffHandler(SESSION_PRESETS.admin, {
        origin: "https://admin.example",
        peek: gateway.peek,
        redeem: gateway.redeem,
      }),
    ).toThrow();
  });

  it("buyer and seller presets enable hand-off", () => {
    expect(() => seller()).not.toThrow();
    expect(() =>
      createHandoffHandler(SESSION_PRESETS.buyer, {
        origin: "https://buyer.example",
        peek: gateway.peek,
        redeem: gateway.redeem,
      }),
    ).not.toThrow();
  });
});

describe("logging", () => {
  it("never logs the code", async () => {
    const spies = [
      vi.spyOn(console, "log").mockImplementation(() => {}),
      vi.spyOn(console, "info").mockImplementation(() => {}),
      vi.spyOn(console, "warn").mockImplementation(() => {}),
      vi.spyOn(console, "error").mockImplementation(() => {}),
      vi.spyOn(console, "debug").mockImplementation(() => {}),
    ];
    const handler = seller();
    gateway.mint(CODE, "alice");
    await handler.GET(getReq(`?code=${CODE}&next=/seller`));
    await handler.POST(
      postReq(
        { code: CODE, next: "/seller" },
        { origin: "https://evil.example" },
      ),
    );
    await handler.POST(postReq({ code: CODE, next: "/seller" }, sameOrigin));
    await handler.POST(postReq({ code: CODE, next: "/seller" }, sameOrigin));
    gateway.outage = true;
    await handler.GET(getReq(`?code=${CODE}`));
    expect(consoleOutput(spies)).not.toContain(CODE);
  });
});
