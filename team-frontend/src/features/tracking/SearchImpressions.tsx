"use client";

import { useEffect } from "react";

import { track } from "@/lib/track";

/**
 * Fires a best-effort `impression` beacon for each rendered search result, with
 * its 1-based result position and the active query. Rendered on the search page
 * alongside the results grid. Renders nothing.
 */
export function SearchImpressions({
  listingIds,
  query,
}: {
  listingIds: string[];
  query?: string;
}) {
  useEffect(() => {
    listingIds.forEach((listingId, index) => {
      track({ type: "impression", listingId, position: index + 1, query });
    });
  }, [listingIds, query]);
  return null;
}
