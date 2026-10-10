import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import {
  type ViewOrder,
  getOrder,
  listSellerOrders,
} from "@/lib/gateway/orders";
import type { SessionPrincipal } from "@/lib/gateway/session";
import { requestCache } from "@/lib/requestCache";

/**
 * Seller view of one order: getOrder first; if the gateway does not let a
 * seller principal read it, fall back to the seller's own order list. One read
 * per request, shared by the segment layout and the page.
 */
export const loadSellerOrder = requestCache(
  async (id: string): Promise<ViewOrder | null> => {
    const direct = await getOrder(id);
    if (direct) return direct;
    const mine = await listSellerOrders(OrderStatus.UNSPECIFIED);
    return mine.find((o) => o.id === id) ?? null;
  },
);

export function isOwnedBy(
  order: ViewOrder,
  me: SessionPrincipal | null,
): boolean {
  return (
    me !== null &&
    (order.sellerId === me.id || me.scopes.includes("order.admin"))
  );
}
