import "server-only";

import { getListing, type ViewListing } from "@/lib/gateway/listings";
import { listSponsoredSlotSet } from "@/lib/gateway/promotion";

import { MAX_SPONSORED_SLOTS, SPONSORED_TIMEOUT_MS } from "./constants";

export interface SponsoredItem {
  listing: ViewListing;
  impressionToken: string;
}

async function load(
  placement: string,
  categoryId: string,
): Promise<SponsoredItem[]> {
  const slots = await listSponsoredSlotSet({
    placement,
    categoryId,
    limit: MAX_SPONSORED_SLOTS,
  });
  const items = await Promise.all(
    slots.map(async (s) => {
      const listing = await getListing(s.listingId);
      return listing && listing.status === "published"
        ? { listing, impressionToken: s.impressionToken }
        : null;
    }),
  );
  return items
    .filter((i): i is SponsoredItem => i !== null)
    .slice(0, MAX_SPONSORED_SLOTS);
}

export async function loadSponsored(
  placement: string,
  categoryId: string,
): Promise<SponsoredItem[]> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<SponsoredItem[]>((resolve) => {
    timer = setTimeout(() => resolve([]), SPONSORED_TIMEOUT_MS);
  });
  try {
    return await Promise.race([load(placement, categoryId), timeout]);
  } catch {
    return [];
  } finally {
    clearTimeout(timer);
  }
}
