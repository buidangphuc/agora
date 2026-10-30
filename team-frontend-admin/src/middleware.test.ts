// @vitest-environment node
import { Code, ConnectError } from "@connectrpc/connect";
import { NextRequest } from "next/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

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

function req(cookie: string, headers: Record<string, string> = {}) {
  return new NextRequest("http://localhost/admin/users?q=1", {
    headers: cookie ? { ...headers, cookie } : headers,
  });
}

beforeEach(() => {
  refreshSession.mockReset();
});

describe("admin middleware", () => {
  it("runs on the node runtime and skips robots.txt", () => {
    expect(config.runtime).toBe("nodejs");
    expect(config.matcher[0]).toContain("robots.txt");
  });

  it("forwards the path for the login return url", async () => {
    const res = await middleware(req(""));
    expect(res.headers.get("x-middleware-request-x-pathname")).toBe(
      "/admin/users?q=1",
    );
  });

  it("refreshes an expired admin session into strict admin cookies", async () => {
    const access = jwt(nowSec() + 1800, "u2");
    refreshSession.mockResolvedValue({ token: access, refreshToken: "R2" });
    const res = await middleware(
      req(`admin_session=${jwt(nowSec() - 60)}; admin_refresh=R1`),
    );
    expect(refreshSession).toHaveBeenCalledWith("R1", expect.anything());
    expect(res.cookies.get("admin_session")).toEqual(
      expect.objectContaining({
        value: access,
        httpOnly: true,
        sameSite: "strict",
        maxAge: 1800,
      }),
    );
    expect(res.cookies.get("admin_refresh")).toEqual(
      expect.objectContaining({
        value: "R2",
        sameSite: "strict",
        maxAge: 28800,
      }),
    );
  });

  it("ignores the buyer session cookies", async () => {
    const res = await middleware(
      req(`session=${jwt(nowSec() - 60)}; agora_refresh=R1`),
    );
    expect(refreshSession).not.toHaveBeenCalled();
    expect(res.cookies.get("session")).toBeUndefined();
    expect(res.cookies.get("admin_session")).toBeUndefined();
  });

  it("uses __Host- names behind HTTPS", async () => {
    refreshSession.mockResolvedValue({
      token: jwt(nowSec() + 1800),
      refreshToken: "R2",
    });
    const res = await middleware(
      req("__Host-admin_refresh=R1", { "x-forwarded-proto": "https" }),
    );
    expect(refreshSession).toHaveBeenCalledWith("R1", expect.anything());
    expect(res.cookies.get("__Host-admin_session")?.secure).toBe(true);
  });

  it("clears both cookies when the gateway rejects the refresh token", async () => {
    refreshSession.mockRejectedValue(
      new ConnectError("revoked", Code.Unauthenticated),
    );
    const res = await middleware(
      req(`admin_session=${jwt(nowSec() - 60)}; admin_refresh=R1`),
    );
    expect(res.cookies.get("admin_session")?.value).toBe("");
    expect(res.cookies.get("admin_refresh")?.value).toBe("");
  });
});
