"use client";

import Link from "next/link";
import type { ComponentProps, MouseEvent } from "react";

import { track } from "@/lib/track";

type TrackLinkProps = ComponentProps<typeof Link> & {
  /** Listing this link points at; carried on the click beacon. */
  listingId: string;
  placementId?: string;
  impressionId?: string;
  modelVersion?: string;
  position?: number;
};

/**
 * A drop-in replacement for next/link that fires a best-effort `click` beacon
 * before navigating. Used by the listing card so clicking through to a PDP
 * records the click. The beacon never blocks navigation.
 */
export function TrackLink({
  listingId,
  placementId,
  impressionId,
  modelVersion,
  position,
  onClick,
  children,
  ...rest
}: TrackLinkProps) {
  function handleClick(e: MouseEvent<HTMLAnchorElement>) {
    track({
      type: "click",
      listingId,
      placementId,
      impressionId,
      modelVersion,
      position,
    });
    onClick?.(e);
  }
  return (
    <Link {...rest} onClick={handleClick}>
      {children}
    </Link>
  );
}

