import "server-only";

import { ga4UserId } from "@/lib/analytics/ga4/userId";
import { readConsent } from "@/lib/consent/consent";
import { gatewayConfig } from "@/lib/gateway/config";
import { getToken } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

const NO_STORE = { "Cache-Control": "no-store" };

const none = () => new Response(null, { status: 204, headers: NO_STORE });

// The principal id comes from the gateway, which verifies the token; the
// cookie payload is never trusted here.
async function verifiedPrincipalId(token: string): Promise<string | undefined> {
  try {
    const res = await fetch(
      `${gatewayConfig.gatewayUrl}/api/session/principal`,
      {
        headers: { authorization: `Bearer ${token}` },
        cache: "no-store",
      },
    );
    if (res.status !== 200) return undefined;
    const body = (await res.json()) as { principalId?: unknown };
    return typeof body.principalId === "string" && body.principalId
      ? body.principalId
      : undefined;
  } catch {
    return undefined;
  }
}

export async function GET(request: Request): Promise<Response> {
  if (!readConsent(request.headers.get("cookie") ?? "").analytics) {
    return none();
  }
  const token = getToken();
  if (!token) return none();
  const principalId = await verifiedPrincipalId(token);
  if (!principalId) return none();
  const userId = ga4UserId(principalId);
  if (!userId) return none();
  return Response.json({ userId }, { headers: NO_STORE });
}
