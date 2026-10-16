import { describe, expect, it } from "vitest";

import {
  REFRESH_COOKIE,
  SESSION_COOKIE,
  accessCookieOptions,
  isSecureRequest,
  refreshCookieOptions,
} from "./session-cookie";

// Assumed API: accessCookieOptions(secure) / refreshCookieOptions(secure) return
// the options object passed to cookies().set / NextResponse cookies.set.
describe("session cookie helpers", () => {
  it("S1: names the cookies session and agora_refresh", () => {
    expect(SESSION_COOKIE).toBe("session");
    expect(REFRESH_COOKIE).toBe("agora_refresh");
  });

  it("S1: access cookie lasts 3600 s, httpOnly, lax, path /", () => {
    expect(accessCookieOptions(false)).toEqual(
      expect.objectContaining({
        httpOnly: true,
        sameSite: "lax",
        path: "/",
        maxAge: 3600,
      }),
    );
  });

  it("S1: refresh cookie lasts 2592000 s, httpOnly, lax, path /", () => {
    expect(refreshCookieOptions(false)).toEqual(
      expect.objectContaining({
        httpOnly: true,
        sameSite: "lax",
        path: "/",
        maxAge: 2592000,
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
    expect(isSecureRequest(new Headers({ "x-forwarded-proto": "https" }))).toBe(true);
  });
  it("S2: http forwarded proto is not secure", () => {
    expect(isSecureRequest(new Headers({ "x-forwarded-proto": "http" }))).toBe(false);
  });
  it("S2: absent forwarded proto is not secure", () => {
    expect(isSecureRequest(new Headers())).toBe(false);
  });
});
