import { beforeEach, describe, expect, it, vi } from "vitest";

const headerGet = vi.hoisted(() => vi.fn());
const redirectToLogin = vi.hoisted(() => vi.fn());

vi.mock("next/headers", () => ({
  headers: async () => ({ get: headerGet }),
}));
vi.mock("./returnUrl", () => ({ redirectToLogin }));

import { buildReturnPath, redirectGuestToLoginFromHeader } from "./guardPath";

beforeEach(() => {
  headerGet.mockReset();
  redirectToLogin.mockReset();
  redirectToLogin.mockImplementation(() => {
    throw new Error("NEXT_REDIRECT");
  });
});

describe("Referrer hygiene: non-admin guard still preserves the query", () => {
  it("non-admin query preserved", () => {
    expect(buildReturnPath("/chat", { thread: "t1", tag: ["a", "b"] })).toBe(
      "/chat?thread=t1&tag=a&tag=b",
    );
  });
});

describe("Referrer hygiene: header-based guard drops the admin query", () => {
  it("header guard keeps non-admin query", async () => {
    headerGet.mockImplementation((n: string) =>
      n === "x-pathname" ? "/chat?thread=t1" : null,
    );
    await expect(redirectGuestToLoginFromHeader()).rejects.toThrow();
    expect(redirectToLogin).toHaveBeenCalledWith("/chat?thread=t1");
  });
});
