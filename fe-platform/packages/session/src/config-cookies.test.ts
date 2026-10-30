import { describe, expect, it } from "vitest";
import { SESSION_PRESETS, defineSession } from "./config";
import { cookieNames, cookieOptions, isSecureRequest } from "./cookies";

describe("presets", () => {
  it("keeps the buyer cookie names the running app already uses", () => {
    expect(SESSION_PRESETS.buyer.cookies).toEqual({
      access: "session",
      refresh: "agora_refresh",
    });
    expect(SESSION_PRESETS.buyer.hostPrefix).toBe(false);
  });

  it("(a) gives every app its own pair of cookie names", () => {
    const all = Object.values(SESSION_PRESETS).flatMap((p) => [
      p.cookies.access,
      p.cookies.refresh,
    ]);
    expect(new Set(all).size).toBe(all.length);
  });

  it("requires listing.write for seller and admin for admin", () => {
    expect(SESSION_PRESETS.buyer.requiredScopes).toEqual([]);
    expect(SESSION_PRESETS.seller.requiredScopes).toEqual(["listing.write"]);
    expect(SESSION_PRESETS.admin.requiredScopes).toEqual(["admin"]);
  });

  it("hardens admin: strict, shorter lifetimes", () => {
    const a = SESSION_PRESETS.admin;
    expect(a.sameSite).toBe("strict");
    expect(a.accessMaxAge).toBeLessThan(SESSION_PRESETS.buyer.accessMaxAge);
    expect(a.refreshMaxAge).toBeLessThan(SESSION_PRESETS.buyer.refreshMaxAge);
  });

  it("allows overrides without losing the other cookie name", () => {
    const c = defineSession("seller", { cookies: { access: "s2" } as never });
    expect(c.cookies).toEqual({ access: "s2", refresh: "seller_refresh" });
    expect(c.app).toBe("seller");
  });
});

describe("cookie names and options", () => {
  it("adds __Host- only for prefixed apps over https", () => {
    expect(cookieNames(SESSION_PRESETS.admin, true).access).toBe(
      "__Host-admin_session",
    );
    expect(cookieNames(SESSION_PRESETS.admin, false).access).toBe(
      "admin_session",
    );
    expect(cookieNames(SESSION_PRESETS.buyer, true).access).toBe("session");
  });

  it("never sets a Domain and always httpOnly, path /", () => {
    for (const cfg of Object.values(SESSION_PRESETS)) {
      for (const kind of ["access", "refresh"] as const) {
        const o = cookieOptions(cfg, kind, true);
        expect(Object.keys(o)).not.toContain("domain");
        expect(o).toMatchObject({ httpOnly: true, path: "/", secure: true });
      }
    }
    expect(cookieOptions(SESSION_PRESETS.admin, "access", false).sameSite).toBe(
      "strict",
    );
  });

  it("detects secure requests from env or forwarded proto", () => {
    const h = (v: string | null) => ({ get: () => v });
    expect(isSecureRequest(h(null), { COOKIE_SECURE: "TRUE" })).toBe(true);
    expect(isSecureRequest(h("https"), {})).toBe(true);
    expect(isSecureRequest(h("http"), {})).toBe(false);
  });
});
