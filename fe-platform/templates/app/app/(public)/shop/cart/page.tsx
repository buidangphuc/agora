import { Button, Card, Input, Link } from "@fe-platform/ui";
import { Page } from "../../../../components/Page";
import { toCartView } from "../../../../lib/cart";
import { errorMessage, guarded } from "../../../../lib/guard";
import { removeItem, updateQuantity } from "./actions";

export const dynamic = "force-dynamic";

export default async function CartPage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string }>;
}) {
  const message = errorMessage((await searchParams).error);
  const cart = toCartView(
    (await guarded("/shop/cart", (c) => c.cart.getCart({}))).cart,
  );
  if (cart.empty) {
    return (
      <Page title="Cart">
        {message ? (
          <p role="alert" className="text-text-danger">
            {message}
          </p>
        ) : null}
        <p className="text-text-secondary">
          Your cart is empty. Pick something from the catalogue.
        </p>
        <Link href="/products">Continue shopping</Link>
      </Page>
    );
  }
  return (
    <Page title="Cart">
      {message ? (
        <p role="alert" className="text-text-danger">
          {message}
        </p>
      ) : null}
      <div className="grid grid-cols-1 gap-8 pb-18 md:grid-cols-3 md:pb-0">
        <div className="md:col-span-2">
          <Card>
            <ul className="flex flex-col divide-y divide-border-subtle">
              {cart.lines.map((l) => (
                <li key={l.id} className="flex flex-col gap-4 py-4">
                  <div className="flex items-start justify-between gap-4">
                    <div className="flex flex-col gap-1">
                      <span className="font-semibold">{l.title}</span>
                      {l.variantName ? (
                        <span className="text-sm text-text-secondary">
                          {l.variantName}
                        </span>
                      ) : null}
                    </div>
                    <span className="font-semibold">{l.unitPrice} each</span>
                  </div>
                  <div className="flex flex-wrap items-end gap-3">
                    <form
                      action={updateQuantity}
                      className="flex items-end gap-3"
                    >
                      <input type="hidden" name="itemId" value={l.id} />
                      <Input
                        label="Quantity"
                        name="quantity"
                        type="number"
                        min={1}
                        defaultValue={l.quantity}
                      />
                      <Button type="submit" variant="secondary">
                        Update
                      </Button>
                    </form>
                    <form action={removeItem}>
                      <input type="hidden" name="itemId" value={l.id} />
                      <Button type="submit" variant="ghost">
                        Remove
                      </Button>
                    </form>
                  </div>
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
                <span>Subtotal</span>
                <span className="font-semibold">{cart.subtotal}</span>
              </p>
              <form
                action="/shop/checkout"
                method="get"
                className="flex flex-col gap-3"
              >
                <Button type="submit" variant="primary" size="lg">
                  Continue to checkout
                </Button>
              </form>
              <Link href="/products">Continue shopping</Link>
            </div>
          </Card>
        </div>
      </div>
      <div className="md:hidden">
        <Link href="/products">Continue shopping</Link>
      </div>
      <div className="fixed inset-x-0 bottom-0 border-t border-border-subtle bg-surface-card pb-safe md:hidden">
        <form
          action="/shop/checkout"
          method="get"
          className="flex items-center justify-between gap-4 px-4 py-3"
        >
          <p className="flex flex-col">
            <span className="text-sm text-text-secondary">Subtotal</span>
            <span className="text-xl font-semibold">{cart.subtotal}</span>
          </p>
          <Button type="submit" variant="primary" size="lg">
            Checkout
          </Button>
        </form>
      </div>
    </Page>
  );
}
