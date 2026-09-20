"use client";

import Link from "next/link";
import type { ComponentProps, MouseEvent } from "react";
import { trackEcommerce } from "@/lib/analytics";

type TrackLinkProps = ComponentProps<typeof Link> & {
  /** Listing this link points at; carried on the click beacon. */
  listingId: string;
  placementId?: string;
  impressionId?: string;
  modelVersion?: string;
  position?: number;
  itemListId?: string;
  price?: number;
};

/**
 * A drop-in replacement for next/link that fires a best-effort `select_item` ecommerce
 * event before navigating. Used by the listing card so clicking through to a PDP
 * records the click. The beacon never blocks navigation.
 */
export function TrackLink({
  listingId,
  placementId,
  impressionId,
  modelVersion,
  position,
  itemListId,
  price,
  onClick,
  children,
  ...rest
}: TrackLinkProps) {
  function handleClick(e: MouseEvent<HTMLAnchorElement>) {
    trackEcommerce("select_item", {
      items: [
        {
          itemId: listingId,
          index: position,
          placementId,
          impressionId,
          modelVersion,
          itemListId: itemListId || placementId,
          price,
        },
      ],
    });
    onClick?.(e);
  }
  return (
    <Link {...rest} onClick={handleClick}>
      {children}
    </Link>
  );
}
