import {
  type HeaderReader,
  cookieNames,
  cookieOptions,
  isSecureRequest as platformIsSecureRequest,
} from "@fe-platform/session";

import { sellerSession } from "../session-config";

export const SESSION_COOKIE = sellerSession.cookies.access;
export const REFRESH_COOKIE = sellerSession.cookies.refresh;

export function isSecureRequest(headers: HeaderReader): boolean {
  return platformIsSecureRequest(headers);
}

export function sessionCookieNames(headers: HeaderReader) {
  return cookieNames(sellerSession, isSecureRequest(headers));
}

export function accessCookieOptions(secure: boolean) {
  return cookieOptions(sellerSession, "access", secure);
}

export function refreshCookieOptions(secure: boolean) {
  return cookieOptions(sellerSession, "refresh", secure);
}
