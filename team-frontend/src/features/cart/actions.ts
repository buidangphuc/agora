"use server";

import { revalidatePath } from "next/cache";

import {
  addToCart,
  clearCart,
  removeFromCart,
  updateCartItem,
} from "@/lib/gateway/cart";

export interface CartActionResult {
  ok: boolean;
  message?: string;
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
    return { ok: true, message: "Đã thêm vào giỏ hàng thành công!" };
  } catch (err: unknown) {
    return {
      ok: false,
      message:
        err instanceof Error ? err.message : "Thêm vào giỏ hàng thất bại.",
    };
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
    return { ok: true };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Cập nhật thất bại.",
    };
  }
}

export async function removeFromCartAction(
  itemId: string,
): Promise<CartActionResult> {
  try {
    await removeFromCart(itemId);
    revalidatePath("/cart");
    revalidatePath("/checkout");
    return { ok: true };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Xóa sản phẩm thất bại.",
    };
  }
}

export async function clearCartAction(): Promise<CartActionResult> {
  try {
    await clearCart();
    revalidatePath("/cart");
    revalidatePath("/checkout");
    return { ok: true };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Dọn giỏ hàng thất bại.",
    };
  }
}
