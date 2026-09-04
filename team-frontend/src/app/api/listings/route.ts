import { NextResponse } from "next/server";

import { listListings } from "@/lib/gateway/listings";

// "Xem thêm" endpoint for the consumer feed; runs server-side (token from cookie).
export const dynamic = "force-dynamic";

export async function GET(request: Request): Promise<NextResponse> {
  const url = new URL(request.url);
  try {
    const page = await listListings({
      status: url.searchParams.get("status") ?? "",
      cursor: url.searchParams.get("cursor") ?? "",
    });
    return NextResponse.json(page);
  } catch {
    return NextResponse.json({ items: [], nextCursor: "", total: 0 });
  }
}
