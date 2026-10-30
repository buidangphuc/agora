import { Button, Card, Checkbox, Input, Link, Select } from "@fe-platform/ui";
import { Page } from "../../../../components/Page";
import { toAddressView, toQuoteView } from "../../../../lib/checkout";
import { errorMessage, guarded } from "../../../../lib/guard";
import { paymentOptions } from "../../../../lib/payment-methods";
import { createAddress, placeOrder } from "./actions";

export const dynamic = "force-dynamic";

export default async function CheckoutPage({
  searchParams,
}: {
  searchParams: Promise<{ address?: string; error?: string }>;
}) {
  const sp = await searchParams;
  const message = errorMessage(sp.error);
  const data = await guarded("/shop/checkout", async (c) => {
    const cart = await c.cart.getCart({});
    const addresses = (await c.address.listAddresses({})).addresses.map(
      toAddressView,
    );
    const selected =
      addresses.find((a) => a.id === sp.address) ??
      addresses.find((a) => a.isDefault) ??
      addresses[0];
    const empty = (cart.cart?.items.length ?? 0) === 0;
    if (empty || !selected)
      return { empty, addresses, selected, quote: undefined };
    const quote = toQuoteView(
      await c.order.previewCheckout({ addressId: selected.id }),
    );
    return { empty, addresses, selected, quote };
  });
  if (data.empty) {
    return (
      <Page title="Checkout">
        <p className="text-text-secondary">
          Your cart is empty, so there is nothing to order yet.
        </p>
        <Link href="/products">Back to products</Link>
      </Page>
    );
  }
  return (
    <Page title="Checkout" description="Step 1 of 2: delivery and payment">
      {message ? (
        <p role="alert" className="text-text-danger">
          {message}
        </p>
      ) : null}
      <div className="grid grid-cols-1 gap-8 md:grid-cols-3">
        <div className="flex flex-col gap-8 md:col-span-2">
          {data.addresses.length > 0 ? (
            <Card>
              <div className="flex flex-col gap-4">
                <p className="text-xl font-semibold">Deliver to</p>
                <ul className="flex flex-col gap-3">
                  {data.addresses.map((a) => (
                    <li key={a.id} className="flex flex-col gap-1">
                      <span className="font-semibold">
                        {a.recipient}
                        {a.id === data.selected?.id ? " (selected)" : ""}
                      </span>
                      <span className="text-text-secondary">
                        {a.line} · {a.phone}
                      </span>
                      {a.id === data.selected?.id ? null : (
                        <Link
                          href={`/shop/checkout?address=${encodeURIComponent(a.id)}`}
                        >
                          Deliver here
                        </Link>
                      )}
                    </li>
                  ))}
                </ul>
              </div>
            </Card>
          ) : null}
          {data.selected ? (
            <Card>
              <form action={placeOrder} className="flex flex-col gap-6">
                <input
                  type="hidden"
                  name="addressId"
                  value={data.selected.id}
                />
                <Select
                  label="Payment method"
                  name="paymentMethod"
                  defaultValue={paymentOptions()[0]?.value}
                  options={paymentOptions()}
                />
                <div className="sticky bottom-0 bg-surface-card pb-safe pt-4 md:static">
                  <Button type="submit" variant="primary" size="lg">
                    Place order
                  </Button>
                </div>
              </form>
            </Card>
          ) : null}
          <Card>
            <form action={createAddress} className="flex flex-col gap-6">
              <p className="text-xl font-semibold">
                {data.addresses.length > 0
                  ? "Add another address"
                  : "Add a delivery address"}
              </p>
              <Input
                label="Recipient name"
                name="recipientName"
                autoComplete="name"
                required
              />
              <Input
                label="Phone"
                name="phone"
                type="tel"
                autoComplete="tel"
                required
              />
              <Input
                label="Street address"
                name="street"
                autoComplete="street-address"
                required
              />
              <Input label="Ward" name="ward" />
              <Input label="District" name="district" />
              <Input
                label="City"
                name="city"
                autoComplete="address-level2"
                required
              />
              <Checkbox label="Use as my default address" name="isDefault" />
              <Button type="submit" variant="secondary">
                Save address
              </Button>
            </form>
          </Card>
        </div>
        <div className="md:sticky md:top-6 md:self-start">
          <Card>
            <div className="flex flex-col gap-4">
              <p className="text-xl font-semibold">Order summary</p>
              {data.quote ? (
                <>
                  <ul className="flex flex-col gap-3 text-text-secondary">
                    {data.quote.groups.map((g) => (
                      <li key={g.sellerId} className="flex flex-col gap-1">
                        <span className="flex justify-between gap-4">
                          <span>Items</span>
                          <span>{g.itemsSubtotal}</span>
                        </span>
                        <span className="flex justify-between gap-4">
                          <span>Shipping</span>
                          <span>{g.freeShipping ? "Free" : g.shippingFee}</span>
                        </span>
                      </li>
                    ))}
                  </ul>
                  {data.quote.shippingQuoted ? null : (
                    <p className="text-sm text-text-secondary">
                      Shipping will be confirmed when the order is placed.
                    </p>
                  )}
                  <p className="flex justify-between text-xl">
                    <span>Total</span>
                    <span className="font-semibold">{data.quote.total}</span>
                  </p>
                </>
              ) : (
                <p className="text-text-secondary">
                  Add a delivery address to see the total.
                </p>
              )}
              <Link href="/shop/cart">Back to cart</Link>
            </div>
          </Card>
        </div>
      </div>
    </Page>
  );
}
