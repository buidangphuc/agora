import { createSessionMiddleware } from "@fe-platform/session";
import type { NextRequest } from "next/server";

import { logAccess } from "@/lib/access-log";
import { clientContextFrom } from "@/lib/gateway/client-context";
import { refreshSession } from "@/lib/gateway/refresh";
import { sellerSession } from "@/lib/session-config";

const sessionMiddleware = createSessionMiddleware(sellerSession, {
  refresh: (refreshToken, client) => refreshSession(refreshToken, client),
  clientContext: clientContextFrom,
  pathnameHeader: "x-pathname",
});

export function middleware(req: NextRequest) {
  logAccess(req);
  return sessionMiddleware(req);
}

export const config = {
  runtime: "nodejs",
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
