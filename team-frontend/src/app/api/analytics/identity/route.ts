import { ga4UserId } from "@/lib/analytics/ga4/userId";
import { readConsent } from "@/lib/consent/consent";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

const NO_STORE = { "Cache-Control": "no-store" };

const none = () => new Response(null, { status: 204, headers: NO_STORE });

export async function GET(request: Request): Promise<Response> {
  if (!readConsent(request.headers.get("cookie") ?? "").analytics) {
    return none();
  }
  const principal = getPrincipal();
  if (!principal?.id) return none();
  const userId = ga4UserId(principal.id);
  if (!userId) return none();
  return Response.json({ userId }, { headers: NO_STORE });
}
