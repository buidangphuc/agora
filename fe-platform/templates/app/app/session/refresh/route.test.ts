import { ApiError } from "@fe-platform/api-client";
import { NextRequest } from "next/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

const refreshSession = vi.fn();
const jar = { get: vi.fn(), set: vi.fn(), delete: vi.fn() };
const setSession = vi.fn();

vi.mock("@fe-platform/api-client", async (orig) => ({
  ...(await orig<typeof import("@fe-platform/api-client")>()),
  createGatewayClients: () => ({ auth: { refreshSession } }),
}));
vi.mock("next/headers", () => ({ cookies: async () => jar }));
vi.mock("../../../lib/session", () => ({
  SESSION_COOKIE: "session",
  REFRESH_COOKIE: "refresh",
  REFRESHED_COOKIE: "refreshed",
  safeNext: (n?: string | null) =>
    n?.startsWith("/") && !n.startsWith("//") ? n : "/",
  setSession: (...a: unknown[]) => setSession(...a),
}));

import { GET } from "./route";

const req = (next: string) =>
  new NextRequest(
    `http://app.test/session/refresh?next=${encodeURIComponent(next)}`,
  );
const cookies = (o: Record<string, string>) =>
  jar.get.mockImplementation((k: string) =>
    o[k] ? { value: o[k] } : undefined,
  );

beforeEach(() => {
  refreshSession.mockReset();
  jar.get.mockReset();
  jar.set.mockReset();
  jar.delete.mockReset();
  setSession.mockReset();
});

describe("refresh route", () => {
  it("rotates the tokens and returns to the page", async () => {
    cookies({ refresh: "r1" });
    refreshSession.mockResolvedValue({
      result: { token: "t2", refreshToken: "r2" },
    });
    const res = await GET(req("/shop/cart"));
    expect(res.headers.get("location")).toBe("http://app.test/shop/cart");
    expect(refreshSession).toHaveBeenCalledWith({ refreshToken: "r1" });
    expect(setSession).toHaveBeenCalledWith(
      expect.objectContaining({ token: "t2", refreshToken: "r2" }),
    );
    expect(jar.set).toHaveBeenCalledWith(
      "refreshed",
      "1",
      expect.objectContaining({ maxAge: 10 }),
    );
  });

  it("sends the user to login when there is no refresh token", async () => {
    cookies({});
    const res = await GET(req("/shop/cart"));
    expect(res.headers.get("location")).toBe(
      "http://app.test/login?next=%2Fshop%2Fcart",
    );
    expect(jar.delete).toHaveBeenCalledWith("session");
  });

  it("sends the user to login when the refresh is refused", async () => {
    cookies({ refresh: "r1" });
    refreshSession.mockRejectedValue(
      new ApiError("unauthenticated", "expired"),
    );
    const res = await GET(req("/dashboard"));
    expect(res.headers.get("location")).toContain("/login?next=%2Fdashboard");
  });

  it("does not loop when it just refreshed", async () => {
    cookies({ refresh: "r1", refreshed: "1" });
    const res = await GET(req("/shop/cart"));
    expect(res.headers.get("location")).toContain("/login");
    expect(refreshSession).not.toHaveBeenCalled();
  });

  it("ignores an external next", async () => {
    cookies({ refresh: "r1" });
    refreshSession.mockResolvedValue({
      result: { token: "t2", refreshToken: "" },
    });
    const res = await GET(req("//evil.test"));
    expect(res.headers.get("location")).toBe("http://app.test/");
  });
});
