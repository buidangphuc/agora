import { createGatewayClients, toApiError } from "@fe-platform/api-client";
import { cookies } from "next/headers";
import { type NextRequest, NextResponse } from "next/server";
import {
  REFRESHED_COOKIE,
  REFRESH_COOKIE,
  SESSION_COOKIE,
  safeNext,
  setSession,
} from "../../../lib/session";

export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const next = safeNext(req.nextUrl.searchParams.get("next"));
  const jar = await cookies();
  const toLogin = () => {
    jar.delete(SESSION_COOKIE);
    jar.delete(REFRESH_COOKIE);
    return NextResponse.redirect(
      new URL(`/login?next=${encodeURIComponent(next)}`, req.url),
    );
  };
  const refreshToken = jar.get(REFRESH_COOKIE)?.value;
  if (!refreshToken || jar.get(REFRESHED_COOKIE)) return toLogin();
  let result:
    | Awaited<
        ReturnType<
          ReturnType<typeof createGatewayClients>["auth"]["refreshSession"]
        >
      >["result"]
    | undefined;
  try {
    result = (
      await createGatewayClients().auth.refreshSession({ refreshToken })
    ).result;
  } catch (err) {
    toApiError(err);
  }
  if (!result?.token) return toLogin();
  await setSession({
    token: result.token,
    refreshToken: result.refreshToken || refreshToken,
    refreshExpiresAt: result.refreshExpiresAt,
  });
  jar.set(REFRESHED_COOKIE, "1", {
    httpOnly: true,
    sameSite: "lax",
    path: "/",
    maxAge: 10,
  });
  return NextResponse.redirect(new URL(next, req.url));
}
