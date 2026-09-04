import { CartView } from "@/features/cart/CartView";
import { isCheckoutEnabled } from "@/lib/flags";
import { getCart } from "@/lib/gateway/cart";

export const dynamic = "force-dynamic";

export default async function CartPage() {
  // Evaluate the checkout kill-switch server-side; the browser only receives the
  // resolved boolean, never the flag SDK or the Flipt endpoint.
  const [cart, checkoutEnabled] = await Promise.all([
    getCart(),
    isCheckoutEnabled(),
  ]);

  return (
    <section className="py-2">
      <CartView initialCart={cart} checkoutEnabled={checkoutEnabled} />
    </section>
  );
}
