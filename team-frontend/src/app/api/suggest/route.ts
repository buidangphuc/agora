import { NextResponse } from "next/server";

import { suggest } from "@/lib/gateway/listings";

// Type-ahead endpoint the client SearchBar calls; runs server-side so the
// gateway bearer never reaches the browser.
export const dynamic = "force-dynamic";

export async function GET(request: Request): Promise<NextResponse> {
  const q = new URL(request.url).searchParams.get("q") ?? "";
  try {
    return NextResponse.json({ suggestions: await suggest(q) });
  } catch {
    return NextResponse.json({ suggestions: [] });
  }
}
