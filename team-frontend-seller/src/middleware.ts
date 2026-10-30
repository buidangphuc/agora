import { createSessionMiddleware } from "@fe-platform/session";

import { clientContextFrom } from "@/lib/gateway/client-context";
import { refreshSession } from "@/lib/gateway/refresh";
import { sellerSession } from "@/lib/session-config";

export const middleware = createSessionMiddleware(sellerSession, {
  refresh: (refreshToken, client) => refreshSession(refreshToken, client),
  clientContext: clientContextFrom,
  pathnameHeader: "x-pathname",
});

export const config = {
  runtime: "nodejs",
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
