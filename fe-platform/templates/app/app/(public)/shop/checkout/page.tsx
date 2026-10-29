import { Button, Input } from "@fe-platform/ui";
import Link from "next/link";
import {
  SHOP_CURRENCY,
  cartQuery,
  cartTotalMinor,
  formatMoney,
  parseCart,
  serializeCart,
} from "../../../../lib/shop";

export default async function CheckoutPage({
  searchParams,
}: {
  searchParams: Promise<{ cart?: string | string[] }>;
}) {
  const lines = parseCart((await searchParams).cart);
  const query = cartQuery(lines);
  if (lines.length === 0) {
    return (
      <main className="flex flex-col gap-4 p-6">
        <h1 className="text-2xl">Checkout</h1>
        <p>Your cart is empty, so there is nothing to order yet.</p>
        <Link href="/shop">Back to shop</Link>
      </main>
    );
  }
  return (
    <main className="flex flex-col gap-4 p-6">
      <h1 className="text-2xl">Checkout</h1>
      <p className="text-text-secondary">
        Order total: {formatMoney(cartTotalMinor(lines), SHOP_CURRENCY)}
      </p>
      <form
        action="/shop/confirmation"
        method="get"
        className="flex max-w-md flex-col gap-4"
      >
        <input type="hidden" name="cart" value={serializeCart(lines)} />
        <Input label="Full name" name="name" autoComplete="name" required />
        <Input
          label="Street address"
          name="address"
          autoComplete="street-address"
          required
        />
        <Input
          label="City"
          name="city"
          autoComplete="address-level2"
          required
        />
        <Input
          label="Phone"
          name="phone"
          type="tel"
          autoComplete="tel"
          helpText="We call only if the delivery fails."
          required
        />
        <div className="flex gap-3">
          <Button type="submit" variant="primary">
            Place order
          </Button>
        </div>
      </form>
      <Link href={`/shop/cart${query}`}>Back to cart</Link>
    </main>
  );
}
