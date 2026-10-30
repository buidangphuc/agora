import { type NextRequest, NextResponse } from "next/server";
import type { SessionConfig } from "./config";
import { cookieNames, cookieOptions, isSecureRequest } from "./cookies";
import { isUsableToken } from "./token";

export const CODE_UNAUTHENTICATED = 16;
export const CODE_PERMISSION_DENIED = 7;

export interface RefreshResult {
  token: string;
  refreshToken?: string;
}

export interface ClientContext {
  ip?: string;
  userAgent?: string;
}

export interface SessionMiddlewareDeps {
  refresh(refreshToken: string, client?: ClientContext): Promise<RefreshResult>;
  clientContext?(headers: Headers): ClientContext;
  pathnameHeader?: string;
}

function rewriteCookie(
  header: string | null,
  remove: string[],
  set: Record<string, string> = {},
): string {
  const drop = new Set([...remove, ...Object.keys(set)]);
  const kept = (header ?? "")
    .split(";")
    .map((p) => p.trim())
    .filter((p) => p && !drop.has(p.split("=")[0]));
  for (const [k, v] of Object.entries(set)) kept.push(`${k}=${v}`);
  return kept.join("; ");
}

function isRejection(err: unknown): boolean {
  const e = err as { code?: unknown; kind?: unknown } | null;
  return (
    e?.code === CODE_UNAUTHENTICATED ||
    e?.code === CODE_PERMISSION_DENIED ||
    e?.kind === "unauthenticated" ||
    e?.kind === "forbidden"
  );
}

export function createSessionMiddleware(
  cfg: SessionConfig,
  deps: SessionMiddlewareDeps,
) {
  return async function sessionMiddleware(
    req: NextRequest,
  ): Promise<NextResponse> {
    const headers = new Headers(req.headers);
    if (deps.pathnameHeader) {
      headers.set(
        deps.pathnameHeader,
        req.nextUrl.pathname + req.nextUrl.search,
      );
    }
    const secure = isSecureRequest(req.headers);
    const names = cookieNames(cfg, secure);
    const raw = req.cookies.get(names.access)?.value;
    const refreshToken = req.cookies.get(names.refresh)?.value;
    const cookieHeader = req.headers.get("cookie");

    const next = () => NextResponse.next({ request: { headers } });
    const setCookie = (remove: string[], set?: Record<string, string>) => {
      const c = rewriteCookie(cookieHeader, remove, set);
      if (c) headers.set("cookie", c);
      else headers.delete("cookie");
    };
    const clearBoth = () => {
      setCookie([names.access, names.refresh]);
      const res = next();
      res.cookies.delete(names.access);
      res.cookies.delete(names.refresh);
      return res;
    };

    if (raw && isUsableToken(raw)) return next();

    if (!refreshToken) {
      if (!raw) return next();
      setCookie([names.access]);
      const res = next();
      res.cookies.delete(names.access);
      return res;
    }

    try {
      const result = await deps.refresh(
        refreshToken,
        deps.clientContext?.(req.headers),
      );
      if (!result.token) return clearBoth();
      const set: Record<string, string> = { [names.access]: result.token };
      if (result.refreshToken) set[names.refresh] = result.refreshToken;
      setCookie([], set);
      const res = next();
      res.cookies.set(
        names.access,
        result.token,
        cookieOptions(cfg, "access", secure),
      );
      if (result.refreshToken) {
        res.cookies.set(
          names.refresh,
          result.refreshToken,
          cookieOptions(cfg, "refresh", secure),
        );
      }
      return res;
    } catch (err) {
      if (isRejection(err)) return clearBoth();
      setCookie([names.access]);
      return next();
    }
  };
}
