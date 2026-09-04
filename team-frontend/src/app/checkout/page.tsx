import Link from "next/link";
import { redirect } from "next/navigation";

import { CheckoutView } from "@/features/order/CheckoutView";
import { isCheckoutEnabled } from "@/lib/flags";
import { listAddresses } from "@/lib/gateway/addresses";
import { getCart } from "@/lib/gateway/cart";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function CheckoutPage() {
  const me = getPrincipal();
  if (!me) redirect("/login");

  // Kill-switch: evaluated server-side. When off, render the unavailable notice
  // instead of the checkout form (covers direct navigation to /checkout). The
  // authoritative block still lives in team-order's CreateOrder.
  const checkoutEnabled = await isCheckoutEnabled();
  if (!checkoutEnabled) {
    return (
      <section className="py-2">
        <div className="mx-auto max-w-md rounded-xs border border-gray-100 bg-white p-10 text-center shadow-shopee">
          <p className="text-4xl">🛠️</p>
          <h2 className="mt-4 text-base font-bold text-gray-800">
            Thanh toán tạm thời không khả dụng
          </h2>
          <p className="mt-1 text-xs text-gray-500">
            Chức năng thanh toán đang tạm dừng. Vui lòng thử lại sau ít phút.
          </p>
          <Link
            href="/cart"
            className="mt-6 inline-block rounded-xs bg-brand px-8 py-2.5 text-xs font-bold text-white shadow-md hover:bg-brand-dark uppercase tracking-wider transition"
          >
            Quay lại giỏ hàng
          </Link>
        </div>
      </section>
    );
  }

  const [cart, addresses] = await Promise.all([getCart(), listAddresses()]);

  if (cart.items.length === 0) {
    redirect("/cart");
  }

  return (
    <section className="py-2">
      <CheckoutView cart={cart} addresses={addresses} />
    </section>
  );
}
