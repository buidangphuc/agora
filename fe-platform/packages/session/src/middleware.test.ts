import { NextRequest } from "next/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  FakeIdentity,
  RejectedError,
  jwt,
  nowSec,
} from "./__helpers__/identity";
import { SESSION_PRESETS, defineSession } from "./config";
import { createSessionMiddleware } from "./middleware";

const refresh = vi.fn();
const buyer = createSessionMiddleware(SESSION_PRESETS.buyer, { refresh });

function req(
  cookies: Record<string, string>,
  headers: Record<string, string> = {},
) {
  const cookie = Object.entries(cookies)
    .map(([k, v]) => `${k}=${v}`)
    .join("; ");
  return new NextRequest("http://localhost/x", {
    headers: { ...(cookie ? { cookie } : {}), ...headers },
  });
}
const forwardedCookie = (res: Response) =>
  res.headers.get("x-middleware-request-cookie");
const deleted = (
  res: { cookies: { get(n: string): { value: string } | undefined } },
  n: string,
) => res.cookies.get(n)?.value === "";

const FRESH = () => jwt({ exp: nowSec() + 3600, sub: "u2" });

beforeEach(() => {
  refresh.mockReset();
});

describe("(d) session middleware behaviour", () => {
  it("passes a usable token through without refreshing", async () => {
    const res = await buyer(
      req({ session: jwt({ exp: nowSec() + 3600 }), agora_refresh: "R1" }),
    );
    expect(refresh).not.toHaveBeenCalled();
    expect(res.cookies.getAll()).toHaveLength(0);
  });

  it("strips a malformed token when there is no refresh cookie", async () => {
    const res = await buyer(req({ session: "garbage" }));
    expect(refresh).not.toHaveBeenCalled();
    expect(deleted(res, "session")).toBe(true);
    expect(forwardedCookie(res) ?? "").not.toContain("session=");
  });

  it("does nothing for an anonymous request", async () => {
    const res = await buyer(req({}));
    expect(res.cookies.getAll()).toHaveLength(0);
  });

  it("refreshes an expired token and rewrites both cookies", async () => {
    const token = FRESH();
    refresh.mockResolvedValue({ token, refreshToken: "R2" });
    const res = await buyer(
      req({ session: jwt({ exp: nowSec() - 60 }), agora_refresh: "R1" }),
    );
    expect(refresh).toHaveBeenCalledWith("R1", undefined);
    expect(res.cookies.get("session")?.value).toBe(token);
    expect(res.cookies.get("agora_refresh")?.value).toBe("R2");
    expect(res.cookies.get("session")).toEqual(
      expect.objectContaining({ httpOnly: true, sameSite: "lax", path: "/" }),
    );
    expect(forwardedCookie(res)).toContain(`session=${token}`);
    expect(forwardedCookie(res)).toContain("agora_refresh=R2");
  });

  it("clears both cookies when the refresh is rejected", async () => {
    refresh.mockRejectedValue(new RejectedError("expired"));
    const res = await buyer(req({ session: "bad", agora_refresh: "R1" }));
    expect(deleted(res, "session")).toBe(true);
    expect(deleted(res, "agora_refresh")).toBe(true);
  });

  it("clears both cookies on an ApiError-shaped rejection", async () => {
    refresh.mockRejectedValue({ kind: "forbidden", message: "no" });
    const res = await buyer(req({ session: "bad", agora_refresh: "R1" }));
    expect(deleted(res, "agora_refresh")).toBe(true);
  });

  it("keeps the refresh cookie on a transient failure and renders anonymous", async () => {
    refresh.mockRejectedValue(new Error("network"));
    const res = await buyer(req({ session: "bad", agora_refresh: "R1" }));
    expect(deleted(res, "agora_refresh")).toBe(false);
    expect(forwardedCookie(res) ?? "").not.toContain("session=");
    expect(forwardedCookie(res)).toContain("agora_refresh=R1");
  });

  it("clears both cookies when refresh returns no token", async () => {
    refresh.mockResolvedValue({ token: "" });
    const res = await buyer(req({ session: "bad", agora_refresh: "R1" }));
    expect(deleted(res, "session")).toBe(true);
  });

  it("sets the pathname header and passes client context when configured", async () => {
    const mw = createSessionMiddleware(SESSION_PRESETS.buyer, {
      refresh,
      pathnameHeader: "x-pathname",
      clientContext: () => ({ ip: "1.2.3.4" }),
    });
    refresh.mockResolvedValue({ token: FRESH() });
    const res = await mw(req({ agora_refresh: "R1" }));
    expect(res.headers.get("x-middleware-request-x-pathname")).toBe("/x");
    expect(refresh).toHaveBeenCalledWith("R1", { ip: "1.2.3.4" });
  });

  it("uses prefixed cookie names for admin over https", async () => {
    const admin = createSessionMiddleware(SESSION_PRESETS.admin, { refresh });
    refresh.mockResolvedValue({ token: FRESH(), refreshToken: "R2" });
    const res = await admin(
      req(
        { "__Host-admin_session": "bad", "__Host-admin_refresh": "R1" },
        { "x-forwarded-proto": "https" },
      ),
    );
    expect(res.cookies.get("__Host-admin_session")).toEqual(
      expect.objectContaining({ sameSite: "strict", secure: true }),
    );
    expect(res.cookies.get("admin_session")).toBeUndefined();
  });
});

describe("custom names", () => {
  it("honours overridden cookie names", async () => {
    const cfg = defineSession("buyer", {
      cookies: { access: "a", refresh: "r" },
    });
    const mw = createSessionMiddleware(cfg, { refresh });
    refresh.mockResolvedValue({ token: FRESH(), refreshToken: "R2" });
    const res = await mw(req({ a: "bad", r: "R1" }));
    expect(res.cookies.get("a")).toBeDefined();
    expect(res.cookies.get("r")?.value).toBe("R2");
  });
});

describe("FakeIdentity sanity", () => {
  it("rotates and revokes on reuse", async () => {
    const id = new FakeIdentity();
    const r1 = id.login();
    const first = await id.refresh(r1);
    expect(first.refreshToken).not.toBe(r1);
    await expect(id.refresh(r1)).rejects.toThrow("reused");
    expect(id.revokedCount()).toBe(1);
  });
});
