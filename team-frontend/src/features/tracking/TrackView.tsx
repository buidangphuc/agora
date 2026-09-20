"use client";

import { useEffect } from "react";
import { trackEcommerce } from "@/lib/analytics";

/**
 * Fires a best-effort `view_item` ecommerce event once when the component mounts.
 * Rendered on the product detail page so opening a PDP records a listing view in
 * dataLayer and sends a view beacon. Renders nothing.
 */
export function TrackView({
  listingId,
  path,
  price,
  category,
}: {
  listingId: string;
  path?: string;
  price?: number;
  category?: string;
}) {
  useEffect(() => {
    trackEcommerce("view_item", {
      path,
      items: [
        {
          itemId: listingId,
          price,
          itemCategory: category,
        },
      ],
    });
  }, [listingId, path, price, category]);

  return null;
}
