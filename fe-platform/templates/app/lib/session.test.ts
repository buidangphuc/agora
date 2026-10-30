import { beforeEach, describe, expect, it, vi } from "vitest";

const jar = { get: vi.fn(), set: vi.fn(), delete: vi.fn() };

vi.mock("next/headers", () => ({ cookies: async () => jar }));
vi.mock("next/navigation", () => ({
  redirect: (to: string) => {
    throw new Error(`NEXT_REDIRECT:${to}`);
  },
}));

import {
  REFRESH_COOKIE,
  SESSION_COOKIE,
  clearSession,
  getRefreshToken,
  getToken,
  requireSession,
  safeNext,
  setSession,
} from "./session";

beforeEach(() => {
  jar.get.mockReset();
  jar.set.mockReset();
  jar.delete.mockReset();
});

describe("session", () => {
  it("reads the session cookie", async () => {
    jar.get.mockReturnValue({ value: "tok" });
    await expect(getToken()).resolves.toBe("tok");
    expect(jar.get).toHaveBeenCalledWith(SESSION_COOKIE);
  });

  it("treats a missing or empty cookie as no token", async () => {
    jar.get.mockReturnValue(undefined);
    await expect(getToken()).resolves.toBeUndefined();
    jar.get.mockReturnValue({ value: "" });
    await expect(getToken()).resolves.toBeUndefined();
  });

  it("reads the refresh cookie", async () => {
    jar.get.mockReturnValue({ value: "r1" });
    await expect(getRefreshToken()).resolves.toBe("r1");
    expect(jar.get).toHaveBeenCalledWith(REFRESH_COOKIE);
  });

  it("requireSession redirects to login and keeps the target", async () => {
    jar.get.mockReturnValue(undefined);
    await expect(requireSession("/shop/cart")).rejects.toThrow(
      "NEXT_REDIRECT:/login?next=%2Fshop%2Fcart",
    );
    await expect(requireSession()).rejects.toThrow(
      "NEXT_REDIRECT:/login?next=%2F",
    );
  });

  it("requireSession returns the token when present", async () => {
    jar.get.mockReturnValue({ value: "tok" });
    await expect(requireSession()).resolves.toBe("tok");
  });

  it("stores both tokens as httpOnly cookies and clears them", async () => {
    const expires = new Date("2030-01-01T00:00:00Z");
    await setSession({
      token: "a",
      refreshToken: "r",
      refreshExpiresAt: { toDate: () => expires },
    });
    expect(jar.set).toHaveBeenCalledWith(
      SESSION_COOKIE,
      "a",
      expect.objectContaining({ httpOnly: true, sameSite: "lax", path: "/" }),
    );
    expect(jar.set).toHaveBeenCalledWith(
      REFRESH_COOKIE,
      "r",
      expect.objectContaining({ httpOnly: true, expires }),
    );
    await clearSession();
    expect(jar.delete).toHaveBeenCalledWith(SESSION_COOKIE);
    expect(jar.delete).toHaveBeenCalledWith(REFRESH_COOKIE);
  });

  it("does not store an empty refresh token", async () => {
    await setSession({ token: "a", refreshToken: "" });
    expect(jar.set).toHaveBeenCalledTimes(1);
  });
});

describe("safeNext", () => {
  it("accepts same-origin paths only", () => {
    expect(safeNext("/shop/cart?x=1")).toBe("/shop/cart?x=1");
    for (const bad of [
      undefined,
      null,
      "",
      "https://evil.test",
      "//evil.test",
      "/\\evil",
      "shop",
    ]) {
      expect(safeNext(bad)).toBe("/");
    }
  });
});
