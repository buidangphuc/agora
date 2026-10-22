import { beforeEach, describe, expect, it, vi } from "vitest";

const redirectMock = vi.fn((url: string) => {
  throw new Error(`NEXT_REDIRECT:${url}`);
});
vi.mock("next/navigation", () => ({
  redirect: (url: string) => redirectMock(url),
}));

import { loginPath, redirectToLogin, safeReturnUrl } from "./returnUrl";

const longPath = (n: number) => `/${"a".repeat(n - 1)}`;

describe("safeReturnUrl", () => {
  it("[Same-origin path with query is kept] keeps a same-origin path with query", () => {
    expect(safeReturnUrl("/listing/abc?variant=v2")).toBe(
      "/listing/abc?variant=v2",
    );
  });

  it.each([
    "https://evil.example",
    "//evil",
    "/\\evil",
    "javascript:alert(1)",
    "/\t/evil",
    "/a\nb",
    "/a\u0000b",
    "/a\\b",
  ])(
    "[Absolute, protocol-relative and scheme URLs are rejected] rejects open-redirect shapes %j",
    (raw) => {
      expect(safeReturnUrl(raw)).toBe("/");
    },
  );

  it.each([
    "/a%0d%0ab",
    "/a%0Ab",
    "/a%09b",
    "/a%5Cb",
    "/%5Cevil",
    "/%2F%2Fevil",
    "/%2f%2fevil",
    "/x?next=%0D",
    "/x#%5c",
    "/a%0D%0Ab",
  ])(
    "[Percent-encoded tricks are rejected] rejects percent-encoded tricks %s",
    (raw) => {
      expect(safeReturnUrl(raw)).toBe("/");
    },
  );

  it("[Percent-encoded tricks are rejected] benign encodings are kept", () => {
    expect(safeReturnUrl("/a%20b")).toBe("/a%20b");
    expect(safeReturnUrl("/a%2Fb")).toBe("/a%2Fb");
  });

  it("[Percent-encoded tricks are rejected] /%2F%2F only matters at the start of the path", () => {
    expect(safeReturnUrl("/x/%2F%2Fy")).toBe("/x/%2F%2Fy");
  });

  it.each([
    "/login",
    "/login?returnUrl=%2Fx",
    "/Register",
    "/register/",
    "/LOGIN",
    "/login#x",
    "/login/x",
    "/register?a=1",
    "/Register/Foo",
  ])(
    "[Login and register targets are rejected to avoid a loop] rejects login and register targets %s",
    (raw) => {
      expect(safeReturnUrl(raw)).toBe("/");
    },
  );

  it.each(["/loginfoo", "/registered", "/loginfoo?x=1", "/a/login"])(
    "[Paths merely prefixed by login or register are kept] keeps look-alike prefixes %s",
    (raw) => {
      expect(safeReturnUrl(raw)).toBe(raw);
    },
  );

  it("[Missing, empty and oversize input fall back] falls back on missing and oversize input", () => {
    expect(safeReturnUrl(undefined)).toBe("/");
    expect(safeReturnUrl(null)).toBe("/");
    expect(safeReturnUrl("")).toBe("/");
    expect(safeReturnUrl(longPath(2049))).toBe("/");
    expect(safeReturnUrl(longPath(2048))).toBe(longPath(2048));
  });

  it("[Missing, empty and oversize input fall back] non-string input falls back", () => {
    expect(safeReturnUrl(42 as unknown as string)).toBe("/");
    expect(safeReturnUrl({} as unknown as string)).toBe("/");
  });

  it("[Absolute, protocol-relative and scheme URLs are rejected] relative path without leading slash is rejected", () => {
    expect(safeReturnUrl("listing/abc")).toBe("/");
  });
});

describe("loginPath", () => {
  it("[loginPath encodes the return target] loginPath encodes and guards", () => {
    expect(loginPath("/listing/abc?variant=v2")).toBe(
      "/login?returnUrl=%2Flisting%2Fabc%3Fvariant%3Dv2",
    );
    expect(loginPath("//evil")).toBe("/login");
    expect(loginPath(undefined)).toBe("/login");
  });

  it.each(["https://evil.example", "/login", "/a%0d", "", null])(
    "[loginPath encodes the return target] drops the param when safeReturnUrl(x) !== x: %j",
    (x) => {
      expect(loginPath(x as string | null | undefined)).toBe("/login");
    },
  );

  it("[loginPath encodes the return target] a path that equals '/' is kept as a valid target", () => {
    expect(loginPath("/")).toBe("/login?returnUrl=%2F");
  });

  it("[loginPath encodes the return target] encodes already-encoded input once more", () => {
    expect(loginPath("/a%20b")).toBe("/login?returnUrl=%2Fa%2520b");
  });
});

describe("redirectToLogin", () => {
  beforeEach(() => {
    redirectMock.mockClear();
  });

  it("[Guest opening a protected page is sent to login with its path] redirects to login with the encoded path", () => {
    expect(() => redirectToLogin("/account/orders/o-9")).toThrow(
      /NEXT_REDIRECT/,
    );
    expect(redirectMock).toHaveBeenCalledTimes(1);
    expect(redirectMock).toHaveBeenCalledWith(
      "/login?returnUrl=%2Faccount%2Forders%2Fo-9",
    );
  });

  it("[Query string of the guarded page is preserved with repeated keys in order] repeated query keys re-serialised in order", () => {
    expect(() => redirectToLogin("/chat?thread=t1&tag=a&tag=b")).toThrow();
    expect(redirectMock).toHaveBeenCalledWith(
      "/login?returnUrl=%2Fchat%3Fthread%3Dt1%26tag%3Da%26tag%3Db",
    );
  });

  it("[Over-length path falls back to plain login] redirectToLogin falls back past 2048", () => {
    expect(() => redirectToLogin(longPath(2049))).toThrow();
    expect(redirectMock).toHaveBeenLastCalledWith("/login");

    expect(() => redirectToLogin(longPath(2048))).toThrow();
    expect(redirectMock).toHaveBeenLastCalledWith(loginPath(longPath(2048)));
    expect(redirectMock.mock.lastCall?.[0]).toContain("returnUrl=");
  });

  it("[Param-less layout uses the x-pathname header] undefined path redirects to plain /login", () => {
    expect(() => redirectToLogin(undefined)).toThrow();
    expect(redirectMock).toHaveBeenCalledWith("/login");
  });

  it("[Guest opening a protected page is sent to login with its path] unsafe path redirects to plain /login", () => {
    expect(() => redirectToLogin("//evil")).toThrow();
    expect(redirectMock).toHaveBeenCalledWith("/login");
  });
});
