import type { ViewListing } from "@/lib/gateway/listings";

export function dedupe(
  items: ViewListing[],
  sponsored: { listing: { id: string } }[],
): ViewListing[] {
  const ids = new Set(sponsored.map((s) => s.listing.id));
  return items.filter((l) => !ids.has(l.id));
}
