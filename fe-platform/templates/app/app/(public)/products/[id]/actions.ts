"use server";

import { redirect } from "next/navigation";
import { attempt } from "../../../../lib/guard";

export async function addToCart(formData: FormData): Promise<void> {
  const listingId = formData.get("listingId")?.toString() ?? "";
  const variantId = formData.get("variantId")?.toString() ?? "";
  const back = `/products/${encodeURIComponent(listingId)}`;
  const result = await attempt(back, (c) =>
    c.cart.addToCart({ listingId, variantId, quantity: 1 }),
  );
  if (!result.ok) redirect(`${back}?error=${result.code}`);
  redirect("/shop/cart");
}
