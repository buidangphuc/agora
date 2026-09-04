import "server-only";

import type { Cart, CartItem } from "@/generated/platform/order/v1/order_pb.js";
import { makeClients } from "./client.js";
import { getToken } from "./session.js";

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

export async function getCart(): Promise<ViewCart> {
  try {
    const res = await gateway().cart.getCart({});
    return mapCart(res.cart);
  } catch {
    return { userId: "", items: [], subtotal: 0, totalItems: 0 };
  }
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
