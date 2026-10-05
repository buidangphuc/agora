/**
 * Clears a session cookie the Gateway is certain to reject (expired/malformed).
 * The Gateway answers Unauthenticated to ANY presented-but-invalid bearer, public
 * routes included, so a stale cookie must never reach it. Server Components cannot
 * write cookies, hence this middleware: it strips the cookie from the request the
 * render sees (so this very render is anonymous) and expires it in the browser.
 * getToken() applies the same check as defense in depth.
 */
import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

import { SESSION_COOKIE } from "@/lib/gateway/session-cookie";
import { isUsableToken } from "@/lib/gateway/token-expiry";

export function middleware(req: NextRequest) {
  const raw = req.cookies.get(SESSION_COOKIE)?.value;
  if (!raw || isUsableToken(raw)) return NextResponse.next();

  req.cookies.delete(SESSION_COOKIE);
  const res = NextResponse.next({ request: { headers: req.headers } });
  res.cookies.delete(SESSION_COOKIE);
  return res;
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
