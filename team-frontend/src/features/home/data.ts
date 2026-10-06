import "server-only";

import { cache } from "react";

import { type ViewListing, listListings } from "@/lib/gateway/listings";

export interface HomeFeed {
  items: ViewListing[];
  /** True when the gateway call failed (distinct from "no listings yet"). */
  failed: boolean;
}

/**
 * The home feed, fetched once per request: FeedBlock renders it and the AI
 * assistant reads the same listings, without a second gateway call.
 */
export const loadFeed = cache(async (): Promise<HomeFeed> => {
  try {
    const page = await listListings({ status: "published", pageSize: 24 });
    return { items: page.items, failed: false };
  } catch (err) {
    console.warn("home feed failed", err);
    return { items: [], failed: true };
  }
});
