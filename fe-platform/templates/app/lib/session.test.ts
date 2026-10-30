import { beforeEach, describe, expect, it, vi } from "vitest";

const jar = { get: vi.fn() };

vi.mock("next/headers", () => ({ cookies: async () => jar }));
vi.mock("next/navigation", () => ({
  redirect: (to: string) => {
    throw new Error(`NEXT_REDIRECT:${to}`);
  },
}));

import { SESSION_COOKIE, getToken, requireSession } from "./session";

beforeEach(() => jar.get.mockReset());

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

  it("requireSession redirects when there is no cookie", async () => {
    jar.get.mockReturnValue(undefined);
    await expect(requireSession()).rejects.toThrow("NEXT_REDIRECT:/");
  });

  it("requireSession returns the token when present", async () => {
    jar.get.mockReturnValue({ value: "tok" });
    await expect(requireSession()).resolves.toBe("tok");
  });
});
