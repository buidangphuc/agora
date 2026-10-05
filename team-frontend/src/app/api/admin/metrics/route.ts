import { NextResponse } from "next/server";

import { fetchCockpit } from "@/lib/gateway/cockpit";
import { getToken } from "@/lib/gateway/session";

// Client refresh for /admin/cockpit. The session token is an httpOnly cookie, so
// the browser cannot send a bearer itself; this handler forwards it to the
// gateway, which enforces the `admin` scope (401 / 403 are passed through).
export const dynamic = "force-dynamic";

export async function GET(): Promise<NextResponse> {
  const token = getToken();
  if (!token) {
    return NextResponse.json({ error: "unauthenticated" }, { status: 401 });
  }
  const result = await fetchCockpit(token);
  switch (result.status) {
    case "ok":
      return NextResponse.json(result.data, {
        headers: { "Cache-Control": "no-store" },
      });
    case "unauthenticated":
      return NextResponse.json({ error: "unauthenticated" }, { status: 401 });
    case "forbidden":
      return NextResponse.json({ error: "forbidden" }, { status: 403 });
    default:
      return NextResponse.json({ error: "unavailable" }, { status: 502 });
  }
}
