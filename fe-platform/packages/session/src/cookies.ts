import type { SessionConfig } from "./config";

export interface HeaderReader {
  get(name: string): string | null | undefined;
}

export interface CookieNames {
  access: string;
  refresh: string;
}

export interface CookieOptions {
  httpOnly: true;
  sameSite: "lax" | "strict";
  path: "/";
  maxAge: number;
  secure: boolean;
}

export function isSecureRequest(
  headers: HeaderReader,
  env: Record<string, string | undefined> = process.env,
): boolean {
  if (env.COOKIE_SECURE?.trim().toLowerCase() === "true") return true;
  return headers.get("x-forwarded-proto") === "https";
}

export function cookieNames(cfg: SessionConfig, secure: boolean): CookieNames {
  const prefix = cfg.hostPrefix && secure ? "__Host-" : "";
  return {
    access: `${prefix}${cfg.cookies.access}`,
    refresh: `${prefix}${cfg.cookies.refresh}`,
  };
}

export function cookieOptions(
  cfg: SessionConfig,
  kind: "access" | "refresh",
  secure: boolean,
): CookieOptions {
  return {
    httpOnly: true,
    sameSite: cfg.sameSite,
    path: "/",
    maxAge: kind === "access" ? cfg.accessMaxAge : cfg.refreshMaxAge,
    secure,
  };
}
