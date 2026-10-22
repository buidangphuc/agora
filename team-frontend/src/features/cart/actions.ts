"use server";

import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";

import { type ActionResult, fail, ok } from "@/lib/action-result";
import {
  addToCart,
  clearCart,
  removeFromCart,
  updateCartItem,
} from "@/lib/gateway/cart";

export type CartActionResult =
  | ActionResult<{ itemId: string }>
  | AddToCartUnauthenticated;

/** addToCartAction only: the caller has no session and should go to login. */
export type AddToCartUnauthenticated = {
  ok: false;
  error: string;
  unauthenticated: true;
};

function messageOf(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback;
}

export async function addToCartAction(
  listingId: string,
  variantId?: string,
  quantity = 1,
): Promise<CartActionResult> {
  try {
    const cart = await addToCart(listingId, variantId, quantity);
    revalidatePath("/cart");
    revalidatePath("/checkout");
    const line = cart.items.find(
      (i) => i.listingId === listingId && i.variantId === (variantId ?? ""),
    );
    if (!line) return fail("Không tìm thấy sản phẩm trong giỏ");
    return ok({ itemId: line.id });
  } catch (err: unknown) {
    if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
      return {
        ok: false,
        error: "Chưa đăng nhập",
        unauthenticated: true,
      };
    }
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
