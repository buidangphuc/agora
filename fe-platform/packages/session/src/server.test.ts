import { ApiError } from "@fe-platform/api-client";
import { beforeEach, describe, expect, it, vi } from "vitest";
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

import { createSessionServer, safeNext } from "./server";

const clients = { tag: "clients" };
const buyer = createSessionServer(SESSION_PRESETS.buyer, () => clients);
const seller = createSessionServer(SESSION_PRESETS.seller, () => clients);
const admin = createSessionServer(SESSION_PRESETS.admin, () => clients);

const ok = (scopes: string[]) =>
  jwt({ exp: nowSec() + 3600, scopes, sub: "u1" });

beforeEach(() => {
  jar.store.clear();
  jar.deleted.length = 0;
  jar.secureProto = null;
});

describe("tokens and principal", () => {
  it("reads only a usable token", async () => {
    jar.store.set("session", ok(["a"]));
    await expect(buyer.getToken()).resolves.toBeTruthy();
    expect((await buyer.getPrincipal())?.scopes).toEqual(["a"]);
    jar.store.set("session", jwt({ exp: nowSec() - 10 }));
    await expect(buyer.getToken()).resolves.toBeUndefined();
    await expect(buyer.getPrincipal()).resolves.toBeNull();
  });

  it("requireSession sends an anonymous visitor to login keeping next", async () => {
    await expect(buyer.requireSession("/cart?x=1")).rejects.toThrow(
      "NEXT_REDIRECT:/login?next=%2Fcart%3Fx%3D1",
    );
    await expect(buyer.requireSession("https://evil.test")).rejects.toThrow(
      "next=%2F",
    );
  });
});

describe("(c) scope checks", () => {
  it("lets a seller token into the seller app and a buyer token out", async () => {
    jar.store.set("seller_session", ok(["listing.read", "listing.write"]));
    await expect(seller.requireAccess("/seller")).resolves.toBeTruthy();
    jar.store.set("seller_session", ok(["listing.read", "search:read"]));
    await expect(seller.requireAccess("/seller")).rejects.toThrow(
      "NEXT_REDIRECT:/forbidden",
    );
  });

  it("admin needs the admin scope; a seller token is turned away", async () => {
    jar.store.set("admin_session", ok(["listing.write", "listing.read"]));
    await expect(admin.requireAccess("/")).rejects.toThrow(
      "NEXT_REDIRECT:/forbidden",
    );
    jar.store.set("admin_session", ok(["admin", "order.admin"]));
    await expect(admin.requireAccess("/")).resolves.toBeTruthy();
  });

  it("the buyer app needs no extra scope", async () => {
    jar.store.set("session", ok([]));
    await expect(buyer.requireAccess("/")).resolves.toBeTruthy();
  });
});

describe("guarded and attempt", () => {
  beforeEach(() => jar.store.set("session", ok([])));

  it("passes the clients to the callback", async () => {
    await expect(buyer.guarded("/x", async (c) => c)).resolves.toBe(clients);
  });

  it("maps unauthenticated, forbidden and not_found", async () => {
    await expect(
      buyer.guarded("/x", async () => {
        throw new ApiError("unauthenticated", "no");
      }),
    ).rejects.toThrow("NEXT_REDIRECT:/login?next=%2Fx");
    await expect(
      buyer.guarded("/x", async () => {
        throw new ApiError("forbidden", "no");
      }),
    ).rejects.toThrow("NEXT_REDIRECT:/forbidden");
    await expect(
      buyer.guarded("/x", async () => {
        throw new ApiError("not_found", "no");
      }),
    ).rejects.toThrow("NEXT_NOT_FOUND");
    await expect(
      buyer.guarded("/x", async () => {
        throw new ApiError("unavailable", "down");
      }),
    ).rejects.toMatchObject({ kind: "unavailable" });
  });

  it("attempt returns a value or a code", async () => {
    await expect(buyer.attempt("/x", async () => 5)).resolves.toEqual({
      ok: true,
      value: 5,
    });
    await expect(
      buyer.attempt("/x", async () => {
        throw new ApiError("invalid", "bad");
      }),
    ).resolves.toEqual({ ok: false, code: "invalid" });
    await expect(
      buyer.attempt("/x", async () => {
        throw new ApiError("unauthenticated", "no");
      }),
    ).rejects.toThrow("NEXT_REDIRECT:/login");
  });
});

describe("setSession and clearSession", () => {
  it("writes the app's own names, prefixed over https for admin", async () => {
    await admin.setSession({ token: "t", refreshToken: "r" });
    expect([...jar.store.keys()]).toEqual(["admin_session", "admin_refresh"]);
    jar.store.clear();
    jar.secureProto = "https";
    await admin.setSession({ token: "t", refreshToken: "r" });
    expect([...jar.store.keys()]).toEqual([
      "__Host-admin_session",
      "__Host-admin_refresh",
    ]);
  });

  it("does not write an empty refresh token", async () => {
    await seller.setSession({ token: "t" });
    expect([...jar.store.keys()]).toEqual(["seller_session"]);
  });
});

describe("safeNext", () => {
  it("accepts same-origin paths only", () => {
    expect(safeNext("/a?b=1")).toBe("/a?b=1");
    for (const bad of [undefined, null, "", "//x", "/\\x", "http://x", "a"]) {
      expect(safeNext(bad)).toBe("/");
    }
  });
});
