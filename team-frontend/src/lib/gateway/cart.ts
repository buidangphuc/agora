import "server-only";

import { Code, ConnectError } from "@connectrpc/connect";

import type { Cart, CartItem } from "@/generated/platform/order/v1/order_pb.js";
import { makeClients } from "./client.js";
import { getToken } from "./session.js";
import { batchGetShopNames } from "./shops.js";

function gateway() {
  return makeClients(getToken());
}

export interface ViewCartItem {
  id: string;
  listingId: string;
  variantId: string;
  quantity: number;
  unitPrice: number;
  title: string;
  variantName: string;
  imageUrl: string;
  sellerId: string;
  /** Resolved shop name; empty when unknown (render via shopLabel()). */
  sellerDisplayName: string;
}

export interface ViewCart {
  userId: string;
  items: ViewCartItem[];
  subtotal: number;
  totalItems: number;
}

function mapCartItem(it: CartItem): ViewCartItem {
  return {
    id: it.id,
    listingId: it.listingId,
    variantId: it.variantId,
    quantity: it.quantity,
    unitPrice: Number(it.unitPrice),
    title: it.title,
    variantName: it.variantName,
    imageUrl: it.imageUrl,
    sellerId: it.sellerId,
    sellerDisplayName: "",
  };
}

function mapCart(c?: Cart): ViewCart {
  if (!c) {
    return { userId: "", items: [], subtotal: 0, totalItems: 0 };
  }
  const items = c.items.map(mapCartItem);
  const totalItems = items.reduce((acc, it) => acc + it.quantity, 0);
  return {
    userId: c.userId,
    items,
    subtotal: Number(c.subtotal),
    totalItems,
  };
}

// withShopNames attaches seller display names with ONE batch call. It never
// throws: a failed lookup leaves names empty and the cart still renders.
async function withShopNames(cart: ViewCart): Promise<ViewCart> {
  if (cart.items.length === 0) return cart;
  const names = await batchGetShopNames(cart.items.map((it) => it.sellerId));
  return {
    ...cart,
    items: cart.items.map((it) => ({
      ...it,
      sellerDisplayName: names.get(it.sellerId) ?? "",
    })),
  };
}

const EMPTY_CART: ViewCart = {
  userId: "",
  items: [],
  subtotal: 0,
  totalItems: 0,
};

/**
 * The caller's cart without shop names: one GetCart RPC. Use it where only the
 * items or the item count matter (root layout badge, checkout). An anonymous
 * caller (Unauthenticated) has an empty cart; any other RPC error is a real
 * failure and is thrown so the route's error.tsx can offer a retry, instead of
 * rendering an outage as "cart is empty".
 */
export async function getCart(): Promise<ViewCart> {
  try {
    const res = await gateway().cart.getCart({});
    return mapCart(res.cart);
  } catch (err) {
    if (ConnectError.from(err).code === Code.Unauthenticated) {
      return { ...EMPTY_CART };
    }
    throw err;
  }
}

/**
 * The cart with `sellerDisplayName` resolved (one extra batch call), for pages
 * that render a shop header (/cart). A failed name lookup keeps the cart and
 * leaves the names empty (render via shopLabel()).
 */
export async function getCartWithShopNames(): Promise<ViewCart> {
  return withShopNames(await getCart());
}

/**
 * Reorder: re-add every item of a past order into the caller's cart (team-order
 * resolves the order's lines and merges them in). Returns the updated cart.
 */
export async function reorder(orderId: string): Promise<ViewCart> {
  const res = await gateway().cart.reorder({ orderId });
  return mapCart(res.cart);
}

export async function addToCart(
  listingId: string,
  variantId?: string,
  quantity = 1,
): Promise<ViewCart> {
  const res = await gateway().cart.addToCart({
    listingId,
    variantId: variantId ?? "",
    quantity,
  });
  return mapCart(res.cart);
}

export async function updateCartItem(
  itemId: string,
  quantity: number,
): Promise<ViewCart> {
  const res = await gateway().cart.updateCartItem({
    itemId,
    quantity,
  });
  return mapCart(res.cart);
}

export async function removeFromCart(itemId: string): Promise<ViewCart> {
  const res = await gateway().cart.removeFromCart({
    itemId,
  });
  return mapCart(res.cart);
}

export async function clearCart(): Promise<void> {
  await gateway().cart.clearCart({});
}
