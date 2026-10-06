/**
 * Shop display names. team-domain owns the name (Storefront.display_name); this
 * module resolves many names with ONE BatchGetStorefronts call per 100 ids and
 * builds the single deterministic label every page uses. A name lookup never
 * fails a page: any error (including Unimplemented on an older gateway) yields
 * an empty map and callers fall back to shopLabel().
 */
import "server-only";

import { makeClients } from "./client.js";
import { getToken } from "./session.js";

const BATCH_LIMIT = 100;

/** Resolve sellerId -> trimmed display name for the sellers that have one. */
export async function batchGetShopNames(
  sellerIds: Iterable<string>,
): Promise<Map<string, string>> {
  const names = new Map<string, string>();
  const unique = [...new Set([...sellerIds].filter((id) => id !== ""))];
  if (unique.length === 0) return names;

  const client = makeClients(getToken(), { anonymousFallback: true }).listing;
  for (let i = 0; i < unique.length; i += BATCH_LIMIT) {
    const chunk = unique.slice(i, i + BATCH_LIMIT);
    try {
      const res = await client.batchGetStorefronts({ sellerIds: chunk });
      for (const shop of res.shops) {
        const name = shop.displayName.trim();
        if (name !== "") names.set(shop.sellerId, name);
      }
    } catch (err) {
      console.warn("batchGetShopNames failed; using fallback labels", err);
    }
  }
  return names;
}

/** The only place the fallback label is built. */
export function shopLabel(
  sellerId: string,
  displayName?: string | null,
): string {
  const name = (displayName ?? "").trim();
  if (name !== "") return name;
  if (sellerId === "") return "Shop";
  return `Shop #${sellerId.slice(0, 6)}`;
}
