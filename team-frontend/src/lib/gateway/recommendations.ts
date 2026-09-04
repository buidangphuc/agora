/**
 * Server-side recommendation data functions. They wrap the gateway
 * RecommendationService RPC and map the proto response to plain view models so
 * React Server Components and Server Actions stay free of proto types. The
 * frontend never calls team-ai directly — every call goes through the gateway
 * (Rule 1).
 *
 * The Recommend RPC returns listing ids + scores only (never hydrated cards):
 * recommendation never owns listing content (Rule 3). So we hydrate each id
 * into a full listing card via the existing listing client — the same path
 * `searchListings` uses to resolve search hits — and render with the shared
 * product-card presentation.
 */
import "server-only";

import { Code, ConnectError } from "@connectrpc/connect";

import { RecommendationContext } from "@/generated/platform/recommendation/v1/recommendation_pb.js";

import { makeClients } from "./client.js";
import { type ViewListing, getListing } from "./listings.js";
import { getPrincipal, getToken } from "./session.js";

export { RecommendationContext };

// The recs RPC returns listing ids + scores only (no stock/price), so
// out-of-stock listings are only discovered after we hydrate each id into a
// card. To still fill the requested display count after dropping sold-out
// items, over-fetch from team-ai: ask for `limit * OVERFETCH_FACTOR` ids
// (capped) and trim back to `limit` after the stock filter.
const OVERFETCH_FACTOR = 3;
const OVERFETCH_CAP = 60;

// gateway builds request-scoped clients carrying the caller's session token.
function gateway() {
  return makeClients(getToken());
}

export interface RecommendationsOptions {
  /** Anchor listing for item-to-item contexts (PDP "similar items"). */
  seedListingId?: string;
  /** Where the row is shown; picks the server-side strategy. */
  context?: RecommendationContext;
  /** Max cards to request; 0 = server default. */
  limit?: number;
}

/**
 * Fetch a ranked set of recommendations for the caller and hydrate each into a
 * product card. Returns an empty list when the service is unavailable (e.g.
 * `RECS_ENABLED=false`) so callers can hide the row instead of erroring the
 * whole page.
 */
export async function getRecommendations(
  opts: RecommendationsOptions = {},
): Promise<ViewListing[]> {
  const principal = getPrincipal();
  const limit = opts.limit ?? 10;
  try {
    const res = await gateway().recommendation.recommend({
      userId: principal?.id ?? "",
      anonymousId: "",
      seedListingId: opts.seedListingId ?? "",
      context: opts.context ?? RecommendationContext.UNSPECIFIED,
      // Over-fetch so the stock filter below can still fill `limit` cards.
      limit: Math.min(limit * OVERFETCH_FACTOR, OVERFETCH_CAP),
    });
    // Preserve the model's best-first ranking, hydrate ids → cards, drop any
    // listing that no longer exists (id-only response, Rule 3), drop
    // out-of-stock listings (business filter applied here because hydration
    // already fetched stock for free), then trim to the requested `limit`.
    const ranked = [...res.items].sort((a, b) => a.rank - b.rank);
    const resolved = await Promise.all(
      ranked.map((item) => getListing(item.listingId)),
    );
    return resolved
      .filter((l): l is ViewListing => l !== null && l.stock > 0)
      .slice(0, limit);
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.Unavailable) return [];
    throw err;
  }
}
