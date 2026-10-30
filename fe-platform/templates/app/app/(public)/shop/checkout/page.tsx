import { Button, Card, Input, Link } from "@fe-platform/ui";
import { Page } from "../../../../components/Page";
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
      <Page title="Checkout">
        <p className="text-text-secondary">
          Your cart is empty, so there is nothing to order yet.
        </p>
        <Link href="/shop">Back to shop</Link>
      </Page>
    );
  }
  return (
    <Page title="Checkout" description="Step 1 of 2: delivery details">
      <div className="grid grid-cols-1 gap-8 md:grid-cols-3">
        <div className="md:col-span-2">
          <Card>
            <form action={placeOrder} className="flex flex-col gap-6">
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
                errorText={
                  errors.includes("city") ? "Enter your city." : undefined
                }
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
              <div className="sticky bottom-0 bg-surface-card pb-safe pt-4 md:static">
                <div className="flex flex-col gap-3">
                  <Button type="submit" variant="primary" size="lg">
                    Place order
                  </Button>
                </div>
              </div>
            </form>
          </Card>
        </div>
        <div className="md:sticky md:top-6 md:self-start">
          <Card>
            <div className="flex flex-col gap-4">
              <p className="text-xl font-semibold">Order summary</p>
              <ul className="flex flex-col gap-2 text-text-secondary">
                {lines.map((l) => (
                  <li key={l.product.id} className="flex justify-between gap-4">
                    <span>
                      {l.product.title} × {l.quantity}
                    </span>
                    <span>
                      {formatMoney(l.subtotalMinor, l.product.currency)}
                    </span>
                  </li>
                ))}
              </ul>
              <p className="flex justify-between text-xl">
                <span>Order total</span>
                <span className="font-semibold">
                  {formatMoney(cartTotalMinor(lines), SHOP_CURRENCY)}
                </span>
              </p>
              <Link href={`/shop/cart${query}`}>Back to cart</Link>
            </div>
          </Card>
        </div>
      </div>
    </Page>
  );
}
