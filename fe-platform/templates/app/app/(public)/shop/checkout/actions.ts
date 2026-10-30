"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { cartQuery, parseCart } from "../../../../lib/shop";
import {
  DELIVERY_COOKIE,
  validateDelivery,
} from "../../../../lib/shop-delivery";

export async function placeOrder(formData: FormData): Promise<void> {
  const lines = parseCart(formData.get("cart")?.toString());
  if (lines.length === 0) redirect("/shop");
  const query = cartQuery(lines);
  const result = validateDelivery((k) => formData.get(k));
  if (!result.ok) {
    redirect(`/shop/checkout${query}&error=${result.fields.join(",")}`);
  }
  const jar = await cookies();
  jar.set(DELIVERY_COOKIE, JSON.stringify(result.value), {
    httpOnly: true,
    sameSite: "lax",
    path: "/shop",
    maxAge: 600,
    secure: process.env.NODE_ENV === "production",
  });
  redirect(`/shop/confirmation${query}`);
}
