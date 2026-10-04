"use server";

import { revalidatePath } from "next/cache";

import { type ActionResult, fail, ok } from "@/lib/action-result";
import {
  addToCart,
  clearCart,
  removeFromCart,
  updateCartItem,
} from "@/lib/gateway/cart";

export type CartActionResult = ActionResult;

function messageOf(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback;
}

export async function addToCartAction(
  listingId: string,
  variantId?: string,
  quantity = 1,
): Promise<CartActionResult> {
  try {
    await addToCart(listingId, variantId, quantity);
    revalidatePath("/cart");
    revalidatePath("/checkout");
    return ok();
  } catch (err: unknown) {
    return fail(messageOf(err, "Thêm vào giỏ hàng thất bại."));
  }
}

export async function updateCartItemAction(
  itemId: string,
  quantity: number,
): Promise<CartActionResult> {
  try {
    await updateCartItem(itemId, quantity);
    revalidatePath("/cart");
    revalidatePath("/checkout");
    return ok();
  } catch (err: unknown) {
    return fail(messageOf(err, "Cập nhật thất bại."));
  }
}

export async function removeFromCartAction(
  itemId: string,
): Promise<CartActionResult> {
  try {
    await removeFromCart(itemId);
    revalidatePath("/cart");
    revalidatePath("/checkout");
    return ok();
  } catch (err: unknown) {
    return fail(messageOf(err, "Xóa sản phẩm thất bại."));
  }
}

export async function clearCartAction(): Promise<CartActionResult> {
  try {
    await clearCart();
    revalidatePath("/cart");
    revalidatePath("/checkout");
    return ok();
  } catch (err: unknown) {
    return fail(messageOf(err, "Dọn giỏ hàng thất bại."));
  }
}
