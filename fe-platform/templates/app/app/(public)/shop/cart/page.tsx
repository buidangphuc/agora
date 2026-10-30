import { Button, Card, Link } from "@fe-platform/ui";
import { Page } from "../../../../components/Page";
import {
  SHOP_CURRENCY,
  cartQuery,
  cartTotalMinor,
  formatMoney,
  parseCart,
  serializeCart,
} from "../../../../lib/shop";

export default async function CartPage({
  searchParams,
}: {
  searchParams: Promise<{ cart?: string | string[] }>;
}) {
  const lines = parseCart((await searchParams).cart);
  const query = cartQuery(lines);
  const total = formatMoney(cartTotalMinor(lines), SHOP_CURRENCY);
  if (lines.length === 0) {
    return (
      <Page title="Cart">
        <p className="text-text-secondary">
          Your cart is empty. Pick something from the shop.
        </p>
        <Link href={`/shop${query}`}>Continue shopping</Link>
      </Page>
    );
  }
  return (
    <Page title="Cart">
      <div className="grid grid-cols-1 gap-8 pb-18 md:grid-cols-3 md:pb-0">
        <div className="md:col-span-2">
          <Card>
            <ul className="flex flex-col divide-y divide-border-subtle">
              {lines.map((l) => (
                <li
                  key={l.product.id}
                  className="flex items-center justify-between gap-4 py-4"
                >
                  <div className="flex flex-col gap-1">
                    <span className="font-semibold">{l.product.title}</span>
                    <span className="text-sm text-text-secondary">
                      Quantity: {l.quantity}
                    </span>
                  </div>
                  <span className="font-semibold">
                    {formatMoney(l.subtotalMinor, l.product.currency)}
                  </span>
                </li>
              ))}
            </ul>
          </Card>
        </div>
        <div className="hidden md:sticky md:top-6 md:block md:self-start">
          <Card>
            <div className="flex flex-col gap-6">
              <p className="text-xl font-semibold">Order summary</p>
              <p className="flex justify-between text-xl">
                <span>Total</span>
                <span className="font-semibold">{total}</span>
              </p>
              <form
                action="/shop/checkout"
                method="get"
                className="flex flex-col gap-3"
              >
                <input type="hidden" name="cart" value={serializeCart(lines)} />
                <Button type="submit" variant="primary" size="lg">
                  Continue to checkout
                </Button>
              </form>
              <Link href={`/shop${query}`}>Continue shopping</Link>
            </div>
          </Card>
        </div>
      </div>
      <div className="md:hidden">
        <Link href={`/shop${query}`}>Continue shopping</Link>
      </div>
      <div className="fixed inset-x-0 bottom-0 border-t border-border-subtle bg-surface-card pb-safe md:hidden">
        <form
          action="/shop/checkout"
          method="get"
          className="flex items-center justify-between gap-4 px-4 py-3"
        >
          <input type="hidden" name="cart" value={serializeCart(lines)} />
          <p className="flex flex-col">
            <span className="text-sm text-text-secondary">Total</span>
            <span className="text-xl font-semibold">{total}</span>
          </p>
          <Button type="submit" variant="primary" size="lg">
            Checkout
          </Button>
        </form>
      </div>
    </Page>
  );
}
