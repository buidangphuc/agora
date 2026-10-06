"use client";

import { trackEcommerce } from "@/lib/analytics";
import { useEffect } from "react";

/**
 * Fires a single batched `view_item_list` ecommerce event for the rendered search results,
 * populating dataLayer and sending batched beacons in a single HTTP request to /api/track.
 */
export function SearchImpressions({
  listingIds,
  query,
}: {
  listingIds: string[];
  query?: string;
}) {
  useEffect(() => {
    if (listingIds.length === 0) return;

    trackEcommerce("view_item_list", {
      query,
      items: listingIds.map((listingId, index) => ({
        itemId: listingId,
        index: index + 1,
        itemListId: "search_results",
        itemListName: "Search Results",
      })),
    });
  }, [listingIds, query]);

  return null;
}
