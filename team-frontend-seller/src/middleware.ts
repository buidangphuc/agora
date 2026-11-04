import { createSessionMiddleware } from "@fe-platform/session";
import { type NextRequest, type NextResponse } from "next/server";

import { logAccess } from "@/lib/access-log";
import { clientContextFrom } from "@/lib/gateway/client-context";
import { refreshSession } from "@/lib/gateway/refresh";
import { sellerSession } from "@/lib/session-config";

const sessionMiddleware = createSessionMiddleware(sellerSession, {
  refresh: (refreshToken, client) => refreshSession(refreshToken, client),
  clientContext: clientContextFrom,
  pathnameHeader: "x-pathname",
});

const HANDOFF_HEADERS: Record<string, string> = {
  "content-security-policy":
    "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'",
  "referrer-policy": "no-referrer",
  "cache-control": "no-store",
  "x-frame-options": "DENY",
};

function hardenHandoff(req: NextRequest, res: NextResponse): NextResponse {
  if (req.nextUrl.pathname !== "/auth/handoff") return res;
  for (const [k, v] of Object.entries(HANDOFF_HEADERS)) res.headers.set(k, v);
  return res;
}

export async function middleware(req: NextRequest) {
  logAccess(req);
  return hardenHandoff(req, (await sessionMiddleware(req)) as NextResponse);
}

export const config = {
  runtime: "nodejs",
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
