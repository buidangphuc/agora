"use server";

import { redirect } from "next/navigation";
import { attempt } from "../../../../lib/guard";
import { parsePaymentMethod } from "../../../../lib/payment-methods";

const CHECKOUT = "/shop/checkout";

function withAddress(id: string, error?: string) {
  const q = new URLSearchParams();
  if (id) q.set("address", id);
  if (error) q.set("error", error);
  const s = q.toString();
  return s ? `${CHECKOUT}?${s}` : CHECKOUT;
}

export async function createAddress(formData: FormData): Promise<void> {
  const text = (k: string) => formData.get(k)?.toString().trim() ?? "";
  const result = await attempt(CHECKOUT, (c) =>
    c.address.createAddress({
      recipientName: text("recipientName"),
      phone: text("phone"),
      street: text("street"),
      ward: text("ward"),
      district: text("district"),
      city: text("city"),
      isDefault: formData.get("isDefault") !== null,
    }),
  );
  if (!result.ok) redirect(withAddress("", result.code));
  redirect(withAddress(result.value.address?.id ?? ""));
}

export async function placeOrder(formData: FormData): Promise<void> {
  const addressId = formData.get("addressId")?.toString() ?? "";
  const paymentMethod = parsePaymentMethod(formData.get("paymentMethod"));
  const result = await attempt(CHECKOUT, (c) =>
    c.order.createOrder({ addressId, paymentMethod }),
  );
  if (!result.ok) redirect(withAddress(addressId, result.code));
  const ids = result.value.orders.map((o) => o.id).join(",");
  redirect(`/shop/confirmation?order=${encodeURIComponent(ids)}`);
}
