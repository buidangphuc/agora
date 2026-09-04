/**
 * Web session: the JWT lives in an httpOnly cookie. These helpers read it
 * server-side to attach the bearer and to gate the UI. Decoding here is for
 * DISPLAY/GATING only — the Gateway is the real verification point (ADR-0003);
 * a forged token just gets rejected there.
 */
import "server-only";

import { cookies } from "next/headers";

export const SESSION_COOKIE = "session";

export function getToken(): string | undefined {
  return cookies().get(SESSION_COOKIE)?.value || undefined;
}

export interface SessionPrincipal {
  id: string;
  name: string;
  scopes: string[];
}

/** Decode the session JWT payload (no signature check — display only). */
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
    if (payload.exp && Date.now() / 1000 > payload.exp) return null; // expired
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
