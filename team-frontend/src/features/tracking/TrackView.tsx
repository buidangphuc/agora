"use client";

import { useEffect } from "react";

import { track } from "@/lib/track";

/**
 * Fires a best-effort `view` beacon once when the component mounts. Rendered on
 * the product detail page so opening a PDP records a listing view. Renders
 * nothing.
 */
export function TrackView({
  listingId,
  path,
}: {
  listingId: string;
  path?: string;
}) {
  useEffect(() => {
    track({ type: "view", listingId, path });
  }, [listingId, path]);
  return null;
}
