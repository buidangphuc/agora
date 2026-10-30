import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
  REFRESH_COOKIE,
  SESSION_COOKIE,
  accessCookieOptions,
  isSecureRequest,
  refreshCookieOptions,
  sessionCookieNames,
} from "./session-cookie";

describe("admin session cookie helpers", () => {
  it("names the cookies admin_session and admin_refresh", () => {
    expect(SESSION_COOKIE).toBe("admin_session");
    expect(REFRESH_COOKIE).toBe("admin_refresh");
  });

  it("uses the __Host- prefix on HTTPS", () => {
    expect(
      sessionCookieNames(new Headers({ "x-forwarded-proto": "https" })),
    ).toEqual({
      access: "__Host-admin_session",
      refresh: "__Host-admin_refresh",
    });
    expect(sessionCookieNames(new Headers())).toEqual({
      access: "admin_session",
      refresh: "admin_refresh",
    });
  });

  it("access cookie lasts 1800 s, httpOnly, strict, path /", () => {
    expect(accessCookieOptions(false)).toEqual(
      expect.objectContaining({
        httpOnly: true,
        sameSite: "strict",
        path: "/",
        maxAge: 1800,
      }),
    );
  });

  it("refresh cookie lasts 8 h, httpOnly, strict, path /", () => {
    expect(refreshCookieOptions(false)).toEqual(
      expect.objectContaining({
        httpOnly: true,
        sameSite: "strict",
        path: "/",
        maxAge: 28800,
      }),
    );
  });

  it("S2: secure is mirrored in both cookies (true and false)", () => {
    expect(accessCookieOptions(true).secure).toBe(true);
    expect(refreshCookieOptions(true).secure).toBe(true);
    expect(Boolean(accessCookieOptions(false).secure)).toBe(false);
    expect(Boolean(refreshCookieOptions(false).secure)).toBe(false);
  });
});

// Assumed API: isSecureRequest(headers: Headers): boolean, true iff
// x-forwarded-proto === "https". Used by setSession and the middleware.
describe("isSecureRequest", () => {
  it("S2: https forwarded proto is secure", () => {
    expect(isSecureRequest(new Headers({ "x-forwarded-proto": "https" }))).toBe(
      true,
    );
  });
  it("S2: http forwarded proto is not secure", () => {
    expect(isSecureRequest(new Headers({ "x-forwarded-proto": "http" }))).toBe(
      false,
    );
  });
  it("S2: absent forwarded proto is not secure", () => {
    expect(isSecureRequest(new Headers())).toBe(false);
  });
});

describe("isSecureRequest COOKIE_SECURE", () => {
  const original = process.env.COOKIE_SECURE;
  const restore = () => {
    if (original === undefined)
      Reflect.deleteProperty(process.env, "COOKIE_SECURE");
    else process.env.COOKIE_SECURE = original;
  };
  beforeEach(() => {
    Reflect.deleteProperty(process.env, "COOKIE_SECURE");
  });
  afterEach(restore);

  it("Cookie secure: forced on without proxy header", () => {
    process.env.COOKIE_SECURE = "true";
    expect(isSecureRequest(new Headers())).toBe(true);
  });

  it("Cookie secure: forced on beats a client header (permission)", () => {
    process.env.COOKIE_SECURE = "true";
    expect(isSecureRequest(new Headers({ "x-forwarded-proto": "http" }))).toBe(
      true,
    );
  });

  it("Cookie secure: unset keeps the proxy rule", () => {
    expect(isSecureRequest(new Headers({ "x-forwarded-proto": "https" }))).toBe(
      true,
    );
  });

  it("Cookie secure: unset and plain http (boundary)", () => {
    expect(isSecureRequest(new Headers())).toBe(false);
  });

  it.each(["false", "1", "yes", ""])(
    "Cookie secure: non-true values are ignored (boundary) [%j]",
    (value) => {
      process.env.COOKIE_SECURE = value;
      expect(isSecureRequest(new Headers())).toBe(false);
      expect(
        isSecureRequest(new Headers({ "x-forwarded-proto": "https" })),
      ).toBe(true);
    },
  );

  it("Cookie secure: value is case and space tolerant (boundary)", () => {
    process.env.COOKIE_SECURE = " TRUE ";
    expect(isSecureRequest(new Headers())).toBe(true);
  });

  it("Cookie secure: flag read at request time", () => {
    const h = new Headers();
    expect(isSecureRequest(h)).toBe(false);
    process.env.COOKIE_SECURE = "true";
    expect(isSecureRequest(h)).toBe(true);
    process.env.COOKIE_SECURE = "false";
    expect(isSecureRequest(h)).toBe(false);
  });
});
