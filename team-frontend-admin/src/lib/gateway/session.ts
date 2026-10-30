import "server-only";

import {
  type UnsafeUnwrappedCookies,
  type UnsafeUnwrappedHeaders,
  cookies,
  headers,
} from "next/headers";

import { SESSION_COOKIE, sessionCookieNames } from "./session-cookie.js";
import { isUsableToken } from "./token-expiry.js";

export { SESSION_COOKIE };

export function getToken(): string | undefined {
  const name = sessionCookieNames(
    headers() as unknown as UnsafeUnwrappedHeaders,
  ).access;
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
  const parts = token.split(".");
  if (parts.length !== 3) return null;
  try {
    const json = Buffer.from(
      parts[1].replace(/-/g, "+").replace(/_/g, "/"),
      "base64",
    ).toString("utf8");
    const payload = JSON.parse(json) as {
      sub?: string;
      name?: string;
      scopes?: string[];
      exp?: number;
    };
    if (payload.exp && Date.now() / 1000 > payload.exp) return null;
    return {
      id: payload.sub ?? "",
      name: payload.name ?? "",
      scopes: payload.scopes ?? [],
    };
  } catch {
    return null;
  }
}

export function hasScope(scope: string): boolean {
  return getPrincipal()?.scopes.includes(scope) ?? false;
}
