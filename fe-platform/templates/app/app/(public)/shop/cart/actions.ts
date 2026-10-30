"use server";

import { redirect } from "next/navigation";
import { attempt } from "../../../../lib/guard";

const CART = "/shop/cart";

export async function updateQuantity(formData: FormData): Promise<void> {
  const itemId = formData.get("itemId")?.toString() ?? "";
  const quantity = Number(formData.get("quantity"));
  if (!Number.isInteger(quantity) || quantity < 1)
    redirect(`${CART}?error=invalid`);
  const result = await attempt(CART, (c) =>
    c.cart.updateCartItem({ itemId, quantity }),
  );
  redirect(result.ok ? CART : `${CART}?error=${result.code}`);
}

export async function removeItem(formData: FormData): Promise<void> {
  const itemId = formData.get("itemId")?.toString() ?? "";
  const result = await attempt(CART, (c) => c.cart.removeFromCart({ itemId }));
  redirect(result.ok ? CART : `${CART}?error=${result.code}`);
}
