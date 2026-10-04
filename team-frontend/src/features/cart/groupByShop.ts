import type { ViewCartItem } from "@/lib/gateway/cart";

export interface ShopGroup {
  sellerId: string;
  /** Resolved shop name; empty when unknown (render via shopLabel()). */
  sellerDisplayName: string;
  items: ViewCartItem[];
  subtotal: number;
}

/**
 * Group cart items by seller. Group order is the order in which each seller
 * first appears; item order inside a group is preserved.
 */
export function groupByShop(items: readonly ViewCartItem[]): ShopGroup[] {
  const groups = new Map<string, ShopGroup>();
  for (const item of items) {
    let group = groups.get(item.sellerId);
    if (!group) {
      group = {
        sellerId: item.sellerId,
        sellerDisplayName: item.sellerDisplayName,
        items: [],
        subtotal: 0,
      };
      groups.set(item.sellerId, group);
    }
    if (group.sellerDisplayName === "" && item.sellerDisplayName !== "") {
      group.sellerDisplayName = item.sellerDisplayName;
    }
    group.items.push(item);
    group.subtotal += item.unitPrice * item.quantity;
  }
  return [...groups.values()];
}
