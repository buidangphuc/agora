/**
 * Session continuity at the edge of every request.
 *  - Sets `x-pathname` (path + query, client value overwritten) for the render.
 *  - A session cookie the Gateway is certain to reject (missing/expired/malformed)
 *    is refreshed once via the refresh cookie; on failure it is stripped from the
 *    request the render sees so this render is anonymous. The Gateway answers
 *    Unauthenticated to ANY presented-but-invalid bearer, public routes included.
 * Server Components cannot write cookies, hence this middleware. Tokens are never
 * logged. getToken() applies the same usability check as defense in depth.
 */
import { Code, ConnectError } from "@connectrpc/connect";
import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

import { refreshSession } from "@/lib/gateway/refresh";
import {
  REFRESH_COOKIE,
  SESSION_COOKIE,
  accessCookieOptions,
  isSecureRequest,
  refreshCookieOptions,
} from "@/lib/gateway/session-cookie";
import { isUsableToken } from "@/lib/gateway/token-expiry";

// Rebuilds the forwarded Cookie header: drops `remove` names, then sets `set`.
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

export async function middleware(req: NextRequest) {
  const headers = new Headers(req.headers);
  headers.set("x-pathname", req.nextUrl.pathname + req.nextUrl.search);

  const raw = req.cookies.get(SESSION_COOKIE)?.value;
  const refreshToken = req.cookies.get(REFRESH_COOKIE)?.value;
  const cookieHeader = req.headers.get("cookie");

  const next = () => NextResponse.next({ request: { headers } });
  const setCookie = (remove: string[], set?: Record<string, string>) => {
    const c = rewriteCookie(cookieHeader, remove, set);
    if (c) headers.set("cookie", c);
    else headers.delete("cookie");
  };

  if (raw && isUsableToken(raw)) return next();

  if (!refreshToken) {
    if (!raw) return next();
    setCookie([SESSION_COOKIE]);
    const res = next();
    res.cookies.delete(SESSION_COOKIE);
    return res;
  }

  try {
    const result = await refreshSession(refreshToken);
    if (!result.token) return clearBoth();
    const secure = isSecureRequest(req.headers);
    const set: Record<string, string> = { [SESSION_COOKIE]: result.token };
    if (result.refreshToken) set[REFRESH_COOKIE] = result.refreshToken;
    setCookie([], set);
    const res = next();
    res.cookies.set(SESSION_COOKIE, result.token, accessCookieOptions(secure));
    if (result.refreshToken) {
      res.cookies.set(REFRESH_COOKIE, result.refreshToken, refreshCookieOptions(secure));
    }
    return res;
  } catch (err) {
    if (ConnectError.from(err).code === Code.Unauthenticated) return clearBoth();
    // Transient failure: render anonymous, keep the refresh cookie for a retry.
    setCookie([SESSION_COOKIE]);
    return next();
  }

  function clearBoth() {
    setCookie([SESSION_COOKIE, REFRESH_COOKIE]);
    const res = next();
    res.cookies.delete(SESSION_COOKIE);
    res.cookies.delete(REFRESH_COOKIE);
    return res;
  }
}

export const config = {
  runtime: "nodejs",
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
