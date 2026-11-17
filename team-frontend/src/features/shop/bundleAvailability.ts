import type { ViewBundle } from "@/lib/gateway/listings";

export type BundleAvailability =
  | { available: false; saving?: undefined }
  | { available: true; saving: number | null };

export function bundleAvailability(
  bundle: ViewBundle,
  shopListings: { id: string; price: number }[],
): BundleAvailability {
  const byId = new Map(shopListings.map((l) => [l.id, l]));
  const items = bundle.listingIds.map((id) => byId.get(id));
  if (items.some((l) => l === undefined)) return { available: false };
  const prices = items.map((l) => l?.price ?? 0);
  if (prices.some((p) => !(p > 0))) return { available: true, saving: null };
  const sum = prices.reduce((a, b) => a + b, 0);
  const saving = sum - bundle.bundlePrice;
  return { available: true, saving: saving > 0 ? saving : null };
}
