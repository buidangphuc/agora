"use client";

import { useState } from "react";

import type { ViewListing } from "@/lib/gateway/listings";

import { ListingGrid } from "./ListingGrid";

// ListingFeed renders an SSR first page and appends more via a route handler
// ("Xem thêm"). endpoint is a same-origin URL that returns { items, nextCursor }.
export function ListingFeed({
  initial,
  initialCursor,
  endpoint,
}: {
  initial: ViewListing[];
  initialCursor: string;
  endpoint: string;
}) {
  const [items, setItems] = useState(initial);
  const [cursor, setCursor] = useState(initialCursor);
  const [loading, setLoading] = useState(false);

  async function loadMore() {
    if (!cursor || loading) return;
    setLoading(true);
    try {
      const sep = endpoint.includes("?") ? "&" : "?";
      const res = await fetch(
        `${endpoint}${sep}cursor=${encodeURIComponent(cursor)}`,
      );
      if (res.ok) {
        const page = (await res.json()) as {
          items: ViewListing[];
          nextCursor: string;
        };
        setItems((prev) => [...prev, ...page.items]);
        setCursor(page.nextCursor ?? "");
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <div>
      <ListingGrid listings={items} />
      {cursor && (
        <div className="mt-6 text-center">
          <button
            type="button"
            onClick={loadMore}
            disabled={loading}
            className="rounded-md border px-6 py-2 hover:border-brand disabled:opacity-60"
          >
            {loading ? "Đang tải..." : "Xem thêm"}
          </button>
        </div>
      )}
    </div>
  );
}
