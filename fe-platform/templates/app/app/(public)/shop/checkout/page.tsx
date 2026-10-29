import { Button, Input, Link } from "@fe-platform/ui";
import {
  SHOP_CURRENCY,
  cartQuery,
  cartTotalMinor,
  formatMoney,
  parseCart,
  serializeCart,
} from "../../../../lib/shop";
import { parseErrorFields } from "../../../../lib/shop-delivery";
import { placeOrder } from "./actions";

export default async function CheckoutPage({
  searchParams,
}: {
  searchParams: Promise<{
    cart?: string | string[];
    error?: string | string[];
  }>;
}) {
  const sp = await searchParams;
  const lines = parseCart(sp.cart);
  const errors = parseErrorFields(sp.error);
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
      <form action={placeOrder} className="flex max-w-md flex-col gap-4">
        <input type="hidden" name="cart" value={serializeCart(lines)} />
        <Input
          label="Full name"
          name="name"
          autoComplete="name"
          errorText={
            errors.includes("name") ? "Enter your full name." : undefined
          }
          required
        />
        <Input
          label="Street address"
          name="address"
          autoComplete="street-address"
          errorText={
            errors.includes("address")
              ? "Enter your street address."
              : undefined
          }
          required
        />
        <Input
          label="City"
          name="city"
          autoComplete="address-level2"
          errorText={errors.includes("city") ? "Enter your city." : undefined}
          required
        />
        <Input
          label="Phone"
          name="phone"
          type="tel"
          autoComplete="tel"
          helpText="We call only if the delivery fails."
          errorText={
            errors.includes("phone")
              ? "Enter a phone number with at least 8 digits."
              : undefined
          }
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
