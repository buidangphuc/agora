// @vitest-environment node
import { Code, ConnectError } from "@connectrpc/connect";
import { NextRequest } from "next/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Mock only the true external boundary (the Gateway RefreshSession call).
// Assumed API: middleware(req) is async (returns Promise<NextResponse>); it calls
// refreshSession(refreshToken) with that single argument (no bearer, S12) from
// "@/lib/gateway/refresh"; `config.runtime === "nodejs"`.
const refreshSession = vi.hoisted(() => vi.fn());
vi.mock("@/lib/gateway/refresh", () => ({ refreshSession }));

import { config, middleware } from "./middleware";

function jwt(exp: number, sub = "u1"): string {
  const b = (o: object) =>
    btoa(JSON.stringify(o))
      .replace(/=+$/, "")
      .replace(/\+/g, "-")
      .replace(/\//g, "_");
  return `${b({ alg: "RS256" })}.${b({ sub, exp })}.sig`;
}
const nowSec = () => Math.floor(Date.now() / 1000);

function req(
  opts: {
    session?: string;
    refresh?: string;
    url?: string;
    headers?: Record<string, string>;
  } = {},
) {
  const cookies = [
    opts.session !== undefined ? `session=${opts.session}` : "",
    opts.refresh !== undefined ? `agora_refresh=${opts.refresh}` : "",
  ].filter(Boolean);
  const headers: Record<string, string> = { ...(opts.headers ?? {}) };
  if (cookies.length) headers.cookie = cookies.join("; ");
  return new NextRequest(opts.url ?? "http://localhost/search", { headers });
}

// Request headers the render will see (Next encodes overrides as
// x-middleware-request-<name>).
function forwarded(res: Response, name: string): string | null {
  return res.headers.get(`x-middleware-request-${name}`);
}
function isDeleted(res: { cookies: { get(n: string): any } }, name: string) {
  const c = res.cookies.get(name);
  return c !== undefined && c.value === "";
}

const NEW_ACCESS = jwt(nowSec() + 3600, "u2");
const ok = (accessToken = NEW_ACCESS, refreshToken = "R2") => ({
  token: accessToken,
  refreshToken,
});

beforeEach(() => {
  refreshSession.mockReset();
});

describe("middleware refresh", () => {
  it("S5: expired session + refresh cookie refreshes once and sets rotated cookies", async () => {
    refreshSession.mockResolvedValue(ok());
    const res = await middleware(
      req({ session: jwt(nowSec() - 60), refresh: "R1" }),
    );
    expect(refreshSession).toHaveBeenCalledTimes(1);
    expect(refreshSession).toHaveBeenCalledWith("R1");
    const access = res.cookies.get("session");
    const refresh = res.cookies.get("agora_refresh");
    expect(access?.value).toBe(NEW_ACCESS);
    expect(refresh?.value).toBe("R2");
    expect(access).toEqual(
      expect.objectContaining({ httpOnly: true, sameSite: "lax", path: "/", maxAge: 3600 }),
    );
    expect(refresh).toEqual(
      expect.objectContaining({ httpOnly: true, sameSite: "lax", path: "/", maxAge: 2592000 }),
    );
  });

  it("S6: missing session + refresh cookie behaves as S5", async () => {
    refreshSession.mockResolvedValue(ok());
    const res = await middleware(req({ refresh: "R1" }));
    expect(refreshSession).toHaveBeenCalledTimes(1);
    expect(res.cookies.get("session")?.value).toBe(NEW_ACCESS);
    expect(res.cookies.get("agora_refresh")?.value).toBe("R2");
  });

  it("S2/S5: cookies are secure when x-forwarded-proto is https, not otherwise", async () => {
    refreshSession.mockResolvedValue(ok());
    const https = await middleware(
      req({ refresh: "R1", headers: { "x-forwarded-proto": "https" } }),
    );
    expect(https.cookies.get("session")?.secure).toBe(true);
    expect(https.cookies.get("agora_refresh")?.secure).toBe(true);
    const http = await middleware(req({ refresh: "R1" }));
    expect(Boolean(http.cookies.get("session")?.secure)).toBe(false);
    expect(Boolean(http.cookies.get("agora_refresh")?.secure)).toBe(false);
  });

  it("S21: the render sees the new access and rotated refresh in the Cookie header", async () => {
    refreshSession.mockResolvedValue(ok());
    const res = await middleware(
      req({ session: jwt(nowSec() - 60), refresh: "R1" }),
    );
    const cookie = forwarded(res, "cookie") ?? "";
    expect(cookie).toContain(`session=${NEW_ACCESS}`);
    expect(cookie).toContain("agora_refresh=R2");
    expect(cookie).not.toContain("agora_refresh=R1");
    expect(cookie.match(/session=/g)).toHaveLength(1);
  });

  it("S21: missing session case also shows the new access to the render", async () => {
    refreshSession.mockResolvedValue(ok());
    const res = await middleware(req({ refresh: "R1" }));
    expect(forwarded(res, "cookie")).toContain(`session=${NEW_ACCESS}`);
  });

  it("S7: usable session (with or without refresh) makes zero refresh calls", async () => {
    const live = jwt(nowSec() + 3600);
    const a = await middleware(req({ session: live, refresh: "R1" }));
    const b = await middleware(req({ session: live }));
    expect(refreshSession).not.toHaveBeenCalled();
    expect(a.cookies.get("session")).toBeUndefined();
    expect(b.cookies.get("session")).toBeUndefined();
  });

  it("S7: token just beyond the 5 s skew is usable (boundary)", async () => {
    await middleware(req({ session: jwt(nowSec() + 30), refresh: "R1" }));
    expect(refreshSession).not.toHaveBeenCalled();
  });

  it("S20: exp equal to now triggers exactly one refresh", async () => {
    refreshSession.mockResolvedValue(ok());
    await middleware(req({ session: jwt(nowSec()), refresh: "R1" }));
    expect(refreshSession).toHaveBeenCalledTimes(1);
  });

  it("S20: exp within the 5 s skew triggers exactly one refresh", async () => {
    refreshSession.mockResolvedValue(ok());
    await middleware(req({ session: jwt(nowSec() + 2), refresh: "R1" }));
    expect(refreshSession).toHaveBeenCalledTimes(1);
  });

  it("S8: expired session without refresh cookie clears session, no refresh call", async () => {
    const res = await middleware(req({ session: jwt(nowSec() - 60) }));
    expect(refreshSession).not.toHaveBeenCalled();
    expect(isDeleted(res, "session")).toBe(true);
    expect(forwarded(res, "cookie") ?? "").not.toContain("session=");
  });

  it("S8: no cookies at all makes no refresh call and sets no cookies", async () => {
    const res = await middleware(req());
    expect(refreshSession).not.toHaveBeenCalled();
    expect(res.cookies.get("session")).toBeUndefined();
    expect(res.cookies.get("agora_refresh")).toBeUndefined();
  });

  it("S9: Unauthenticated clears both cookies in browser and render", async () => {
    refreshSession.mockRejectedValue(new ConnectError("bad", Code.Unauthenticated));
    const res = await middleware(
      req({ session: jwt(nowSec() - 60), refresh: "R1" }),
    );
    expect(refreshSession).toHaveBeenCalledTimes(1);
    expect(isDeleted(res, "session")).toBe(true);
    expect(isDeleted(res, "agora_refresh")).toBe(true);
    const cookie = forwarded(res, "cookie") ?? "";
    expect(cookie).not.toContain("session=");
    expect(cookie).not.toContain("agora_refresh=");
  });

  it.each([
    ["Unavailable", new ConnectError("down", Code.Unavailable)],
    ["DeadlineExceeded", new ConnectError("slow", Code.DeadlineExceeded)],
    ["non-Connect error", new Error("boom")],
  ])(
    "S10: %s renders anonymous, leaves both cookies as received, no retry",
    async (_n, err) => {
      refreshSession.mockRejectedValue(err);
      const stale = jwt(nowSec() - 60);
      const res = await middleware(req({ session: stale, refresh: "R1" }));
      expect(refreshSession).toHaveBeenCalledTimes(1);
      expect(res.cookies.get("session")).toBeUndefined();
      expect(res.cookies.get("agora_refresh")).toBeUndefined();
      expect(res.headers.get("set-cookie")).toBeNull();
      // Render is still anonymous: no usable session reaches it, and the
      // refresh cookie is not rewritten.
      const cookie = forwarded(res, "cookie");
      if (cookie !== null) {
        expect(cookie).not.toContain(`session=${stale}`);
        expect(cookie).toContain("agora_refresh=R1");
      }
    },
  );

  it("S11: empty access token is treated as S9", async () => {
    refreshSession.mockResolvedValue(ok("", "R2"));
    const res = await middleware(
      req({ session: jwt(nowSec() - 60), refresh: "R1" }),
    );
    expect(isDeleted(res, "session")).toBe(true);
    expect(isDeleted(res, "agora_refresh")).toBe(true);
    const cookie = forwarded(res, "cookie") ?? "";
    expect(cookie).not.toContain("session=");
    expect(cookie).not.toContain("agora_refresh=");
  });

  it("S12: refreshSession receives only the refresh token (no bearer argument)", async () => {
    refreshSession.mockResolvedValue(ok());
    await middleware(req({ session: jwt(nowSec() - 60), refresh: "R1" }));
    expect(refreshSession.mock.calls[0]).toEqual(["R1"]);
  });

  it("S19: empty refresh_token sets only session and keeps agora_refresh", async () => {
    refreshSession.mockResolvedValue(ok(NEW_ACCESS, ""));
    const res = await middleware(
      req({ session: jwt(nowSec() - 60), refresh: "R1" }),
    );
    expect(res.cookies.get("session")?.value).toBe(NEW_ACCESS);
    expect(res.cookies.get("agora_refresh")).toBeUndefined();
    const cookie = forwarded(res, "cookie") ?? "";
    expect(cookie).toContain(`session=${NEW_ACCESS}`);
    expect(cookie).toContain("agora_refresh=R1");
  });
});

describe("middleware x-pathname", () => {
  it("S23: carries path and query when anonymous", async () => {
    const res = await middleware(req({ url: "http://localhost/cart?coupon=A1" }));
    expect(forwarded(res, "x-pathname")).toBe("/cart?coupon=A1");
  });

  it("S23: carries path and query when signed in", async () => {
    const res = await middleware(
      req({ session: jwt(nowSec() + 3600), url: "http://localhost/cart?coupon=A1" }),
    );
    expect(forwarded(res, "x-pathname")).toBe("/cart?coupon=A1");
  });

  it("S23: carries path and query when refreshed", async () => {
    refreshSession.mockResolvedValue(ok());
    const res = await middleware(
      req({ refresh: "R1", url: "http://localhost/cart?coupon=A1" }),
    );
    expect(forwarded(res, "x-pathname")).toBe("/cart?coupon=A1");
  });

  it("S23: carries path and query when the stale cookie is cleared", async () => {
    const res = await middleware(
      req({ session: jwt(nowSec() - 60), url: "http://localhost/cart?coupon=A1" }),
    );
    expect(forwarded(res, "x-pathname")).toBe("/cart?coupon=A1");
  });

  it("S23: path without query has no trailing question mark", async () => {
    const res = await middleware(req({ url: "http://localhost/search" }));
    expect(forwarded(res, "x-pathname")).toBe("/search");
  });

  it("S26: a client-supplied x-pathname is overwritten with the real path", async () => {
    const res = await middleware(
      req({
        url: "http://localhost/cart?coupon=A1",
        headers: { "x-pathname": "/admin?evil=1" },
      }),
    );
    expect(forwarded(res, "x-pathname")).toBe("/cart?coupon=A1");
  });

  it("S26: overwrite also holds on the refresh path", async () => {
    refreshSession.mockResolvedValue(ok());
    const res = await middleware(
      req({
        refresh: "R1",
        url: "http://localhost/cart",
        headers: { "x-pathname": "/evil" },
      }),
    );
    expect(forwarded(res, "x-pathname")).toBe("/cart");
  });
});

describe("middleware config", () => {
  it("S5: runs on the Node runtime (connect-node client)", () => {
    expect((config as { runtime?: string }).runtime).toBe("nodejs");
  });
  it("keeps the static-asset-excluding matcher", () => {
    expect(config.matcher).toEqual(["/((?!_next/static|_next/image|favicon.ico).*)"]);
  });
});

describe("middleware refresh enforcement (frontend-user-enforcement)", () => {
  it("Users UI: Refresh of a suspended user clears both cookies - permission denied on refresh clears both cookies", async () => {
    refreshSession.mockRejectedValue(
      new ConnectError("user suspended", Code.PermissionDenied),
    );
    const res = await middleware(
      req({ session: jwt(nowSec() - 60), refresh: "R1" }),
    );
    expect(refreshSession).toHaveBeenCalledTimes(1);
    expect(isDeleted(res, "session")).toBe(true);
    expect(isDeleted(res, "agora_refresh")).toBe(true);
    const cookie = forwarded(res, "cookie") ?? "";
    expect(cookie).not.toContain("session=");
    expect(cookie).not.toContain("agora_refresh=");
  });

  it("Users UI: Transient refresh failure keeps the refresh cookie - transient refresh failure keeps the refresh cookie", async () => {
    refreshSession.mockRejectedValue(new ConnectError("down", Code.Unavailable));
    const stale = jwt(nowSec() - 60);
    const res = await middleware(req({ session: stale, refresh: "R1" }));
    expect(res.cookies.get("agora_refresh")).toBeUndefined();
    expect(res.headers.get("set-cookie")).toBeNull();
    const cookie = forwarded(res, "cookie") ?? "";
    expect(cookie).not.toContain(`session=${stale}`);
    expect(cookie).toContain("agora_refresh=R1");
  });
});
