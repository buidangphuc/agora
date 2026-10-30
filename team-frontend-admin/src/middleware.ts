import { createSessionMiddleware } from "@fe-platform/session";
import type { NextRequest } from "next/server";

import { clientContextFrom } from "@/lib/gateway/client-context";
import { refreshSession } from "@/lib/gateway/refresh";
import { adminSession } from "@/lib/gateway/session-cookie";

const session = createSessionMiddleware(adminSession, {
  refresh: async (refreshToken, client) => {
    const result = await refreshSession(refreshToken, client);
    return {
      token: result.token,
      refreshToken: result.refreshToken || undefined,
    };
  },
  clientContext: clientContextFrom,
  pathnameHeader: "x-pathname",
});

export function middleware(req: NextRequest) {
  return session(req);
}

export const config = {
  runtime: "nodejs",
  matcher: ["/((?!_next/static|_next/image|favicon.ico|robots.txt).*)"],
};
