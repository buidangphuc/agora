import { type NextRequest, NextResponse } from "next/server";

import { isSecureRequest } from "@/lib/gateway/session-cookie";
import { resolveShareLink } from "@/lib/gateway/sharing";

export const dynamic = "force-dynamic";

const REFERRAL_COOKIE = "agora_ref";
const REFERRAL_MAX_AGE_S = 604800;
const REFERRAL_ID_RE = /^[A-Za-z0-9_-]{1,32}$/;

function utmQuery(utm: Record<string, string>, lead?: [string, string]) {
  const params = new URLSearchParams();
  if (lead) params.set(lead[0], lead[1]);
  for (const key of Object.keys(utm).sort()) {
    if (key.startsWith("utm_")) params.set(key, utm[key]);
  }
  const q = params.toString();
  return q ? `?${q}` : "";
}

export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ code: string }> },
) {
  const { code } = await params;
  const to = (path: string) =>
    NextResponse.redirect(new URL(path, request.nextUrl), 307);

  let link: Awaited<ReturnType<typeof resolveShareLink>>;
  try {
    link = await resolveShareLink(code);
  } catch {
    return to("/");
  }
  if (!link) return to("/");

  switch (link.targetType) {
    case "listing":
      return to(
        `/listing/${encodeURIComponent(link.targetId)}${utmQuery(link.utm)}`,
      );
    case "shop":
    case "seller":
      return to(
        `/shop/${encodeURIComponent(link.targetId)}${utmQuery(link.utm)}`,
      );
    case "referral": {
      if (!REFERRAL_ID_RE.test(link.targetId)) return to("/");
      const res = to(`/register${utmQuery(link.utm, ["ref", link.targetId])}`);
      res.cookies.set(REFERRAL_COOKIE, link.targetId, {
        httpOnly: true,
        sameSite: "lax",
        path: "/",
        maxAge: REFERRAL_MAX_AGE_S,
        secure: isSecureRequest(request.headers),
      });
      return res;
    }
    default:
      return to("/");
  }
}
