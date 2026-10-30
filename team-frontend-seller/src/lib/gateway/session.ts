import "server-only";

import {
  type HeaderReader,
  decodePrincipal,
  hasScopes,
  isUsableToken,
} from "@fe-platform/session";
import {
  type UnsafeUnwrappedCookies,
  type UnsafeUnwrappedHeaders,
  cookies,
  headers,
} from "next/headers";

import { sellerSession } from "../session-config";
import { SESSION_COOKIE, sessionCookieNames } from "./session-cookie";

export { SESSION_COOKIE };

const NO_HEADERS: HeaderReader = { get: () => null };

function requestHeaders(): HeaderReader {
  try {
    const h = headers() as unknown as UnsafeUnwrappedHeaders;
    return typeof h?.get === "function" ? h : NO_HEADERS;
  } catch {
    return NO_HEADERS;
  }
}

export function getToken(): string | undefined {
  const name = sessionCookieNames(requestHeaders()).access;
  const raw = (cookies() as unknown as UnsafeUnwrappedCookies).get(name)?.value;
  return raw && isUsableToken(raw) ? raw : undefined;
}

export interface SessionPrincipal {
  id: string;
  name: string;
  scopes: string[];
}

export function getPrincipal(): SessionPrincipal | null {
  const token = getToken();
  if (!token) return null;
  const p = decodePrincipal(token);
  return p ? { id: p.id, name: p.name, scopes: p.scopes } : null;
}

export function hasScope(scope: string): boolean {
  return getPrincipal()?.scopes.includes(scope) ?? false;
}

export function hasSellerAccess(principal: SessionPrincipal | null): boolean {
  return hasScopes(principal, sellerSession.requiredScopes);
}
