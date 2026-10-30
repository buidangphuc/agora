export type SessionApp = "buyer" | "seller" | "admin";

export interface SessionConfig {
  app: SessionApp;
  cookies: { access: string; refresh: string };
  hostPrefix: boolean;
  sameSite: "lax" | "strict";
  accessMaxAge: number;
  refreshMaxAge: number;
  requiredScopes: readonly string[];
  loginPath: string;
  forbiddenPath: string;
}

const HOUR = 3600;

export const SESSION_PRESETS: Record<SessionApp, SessionConfig> = {
  buyer: {
    app: "buyer",
    cookies: { access: "session", refresh: "agora_refresh" },
    hostPrefix: false,
    sameSite: "lax",
    accessMaxAge: HOUR,
    refreshMaxAge: 30 * 24 * HOUR,
    requiredScopes: [],
    loginPath: "/login",
    forbiddenPath: "/forbidden",
  },
  seller: {
    app: "seller",
    cookies: { access: "seller_session", refresh: "seller_refresh" },
    hostPrefix: true,
    sameSite: "lax",
    accessMaxAge: HOUR,
    refreshMaxAge: 30 * 24 * HOUR,
    requiredScopes: ["listing.write"],
    loginPath: "/login",
    forbiddenPath: "/forbidden",
  },
  admin: {
    app: "admin",
    cookies: { access: "admin_session", refresh: "admin_refresh" },
    hostPrefix: true,
    sameSite: "strict",
    accessMaxAge: HOUR / 2,
    refreshMaxAge: 8 * HOUR,
    requiredScopes: ["admin"],
    loginPath: "/login",
    forbiddenPath: "/forbidden",
  },
};

export function defineSession(
  app: SessionApp,
  overrides: Partial<Omit<SessionConfig, "app">> = {},
): SessionConfig {
  const base = SESSION_PRESETS[app];
  return {
    ...base,
    ...overrides,
    cookies: { ...base.cookies, ...overrides.cookies },
  };
}
