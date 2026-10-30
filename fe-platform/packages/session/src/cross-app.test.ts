import { NextRequest } from "next/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { FakeIdentity, jwt, nowSec } from "./__helpers__/identity";
import { FakeJar } from "./__helpers__/jar";
import { SESSION_PRESETS } from "./config";
import { createSessionMiddleware } from "./middleware";

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

import { createSessionServer } from "./server";

const identity = new FakeIdentity();

function requestWith(cookies: Record<string, string>) {
  const cookie = Object.entries(cookies)
    .map(([k, v]) => `${k}=${v}`)
    .join("; ");
  return new NextRequest("http://localhost/x", { headers: { cookie } });
}

function appMiddleware(app: "buyer" | "seller" | "admin") {
  return createSessionMiddleware(SESSION_PRESETS[app], {
    refresh: (rt) => identity.refresh(rt),
  });
}

const expired = () => jwt({ exp: nowSec() - 60 });

beforeEach(() => {
  jar.store.clear();
  jar.deleted.length = 0;
  identity.sessions.clear();
});

describe("(a) apps on one host never overwrite each other's cookies", () => {
  it("keeps six distinct cookies when all three apps sign in on localhost", async () => {
    const apps = ["buyer", "seller", "admin"] as const;
    const host = new Map<string, string>();
    for (const app of apps) {
      const cfg = SESSION_PRESETS[app];
      const server = createSessionServer(cfg, () => null);
      await server.setSession({
        token: `access-${app}`,
        refreshToken: `refresh-${app}`,
      });
      for (const [k, v] of jar.store) host.set(k, v);
      jar.store.clear();
    }
    expect(host.size).toBe(6);
    expect(host.get("session")).toBe("access-buyer");
    expect(host.get("seller_session")).toBe("access-seller");
    expect(host.get("admin_session")).toBe("access-admin");
  });
});

describe("(b) refreshing in two apps never revokes either session", () => {
  it("each app rotates its own refresh token concurrently without reuse", async () => {
    const buyerRefresh = identity.login();
    const sellerRefresh = identity.login();
    const [b, s] = await Promise.all([
      appMiddleware("buyer")(
        requestWith({ session: expired(), agora_refresh: buyerRefresh }),
      ),
      appMiddleware("seller")(
        requestWith({
          seller_session: expired(),
          seller_refresh: sellerRefresh,
        }),
      ),
    ]);
    expect(b.cookies.get("agora_refresh")?.value).not.toBe(buyerRefresh);
    expect(s.cookies.get("seller_refresh")?.value).not.toBe(sellerRefresh);
    expect(identity.revokedCount()).toBe(0);
  });

  it("control: two apps sharing one refresh token revoke the session", async () => {
    const shared = identity.login();
    const results = await Promise.all([
      appMiddleware("buyer")(
        requestWith({ session: expired(), agora_refresh: shared }),
      ),
      appMiddleware("buyer")(
        requestWith({ session: expired(), agora_refresh: shared }),
      ),
    ]);
    expect(identity.revokedCount()).toBe(1);
    expect(
      results.some((r) => r.cookies.get("agora_refresh")?.value === ""),
    ).toBe(true);
  });
});

describe("(c) a buyer is turned away by the seller and admin apps", () => {
  const buyerToken = jwt({
    exp: nowSec() + 3600,
    scopes: ["listing.read", "search:read", "search:write", "ai:use"],
  });

  it("redirects to /forbidden in both", async () => {
    for (const app of ["seller", "admin"] as const) {
      const cfg = SESSION_PRESETS[app];
      jar.store.set(cfg.cookies.access, buyerToken);
      const server = createSessionServer(cfg, () => null);
      await expect(server.requireAccess("/")).rejects.toThrow(
        "NEXT_REDIRECT:/forbidden",
      );
    }
  });

  it("is not turned away by the buyer app", async () => {
    jar.store.set("session", buyerToken);
    const server = createSessionServer(SESSION_PRESETS.buyer, () => null);
    await expect(server.requireAccess("/")).resolves.toBeTruthy();
  });
});

describe("(e) signing out of one app leaves the others signed in", () => {
  it("clears only the seller cookies", async () => {
    for (const app of ["buyer", "seller", "admin"] as const) {
      const c = SESSION_PRESETS[app].cookies;
      jar.store.set(c.access, "a");
      jar.store.set(c.refresh, "r");
    }
    await createSessionServer(
      SESSION_PRESETS.seller,
      () => null,
    ).clearSession();
    expect(jar.deleted.sort()).toEqual(["seller_refresh", "seller_session"]);
    expect(jar.store.has("session")).toBe(true);
    expect(jar.store.has("agora_refresh")).toBe(true);
    expect(jar.store.has("admin_session")).toBe(true);
    expect(jar.store.has("admin_refresh")).toBe(true);
  });
});
