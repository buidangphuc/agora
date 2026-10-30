import {
  type CookieNames,
  type HeaderReader,
  cookieNames,
  cookieOptions,
  defineSession,
  isSecureRequest as secureRequest,
} from "@fe-platform/session";

export const adminSession = defineSession("admin");

export const SESSION_COOKIE = adminSession.cookies.access;
export const REFRESH_COOKIE = adminSession.cookies.refresh;

export function isSecureRequest(headers: HeaderReader): boolean {
  return secureRequest(headers);
}

export function sessionCookieNames(headers: HeaderReader): CookieNames {
  return cookieNames(adminSession, isSecureRequest(headers));
}

export function accessCookieOptions(secure: boolean) {
  return cookieOptions(adminSession, "access", secure);
}

export function refreshCookieOptions(secure: boolean) {
  return cookieOptions(adminSession, "refresh", secure);
}
