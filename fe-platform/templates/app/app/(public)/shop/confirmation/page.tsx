import { Card, Link } from "@fe-platform/ui";
import { cookies } from "next/headers";
import { Page } from "../../../../components/Page";
import {
  SHOP_CURRENCY,
  cartTotalMinor,
  formatMoney,
  parseCart,
} from "../../../../lib/shop";
import {
  DELIVERY_COOKIE,
  parseDeliveryCookie,
} from "../../../../lib/shop-delivery";

type Search = Record<string, string | string[] | undefined>;

export default async function ConfirmationPage({
  searchParams,
}: {
  searchParams: Promise<Search>;
}) {
  const sp = await searchParams;
  const lines = parseCart(sp.cart);
  const delivery = parseDeliveryCookie(
    (await cookies()).get(DELIVERY_COOKIE)?.value,
  );
  if (lines.length === 0) {
    return (
      <Page title="No order found" width="narrow">
        <p className="text-text-secondary">
          We could not find an order to show. Start from the shop.
        </p>
        <Link href="/shop">Go to shop</Link>
      </Page>
    );
  }
  return (
    <Page
      title="Order placed"
      description={`${delivery ? `Thank you, ${delivery.name}. ` : "Thank you. "}Your order is on its way.`}
      width="narrow"
    >
      <Card>
        <div className="flex flex-col gap-6">
          <ul className="flex flex-col divide-y divide-border-subtle">
            {lines.map((l) => (
              <li
                key={l.product.id}
                className="flex justify-between gap-4 py-3"
              >
                <span>
                  {l.product.title} × {l.quantity}
                </span>
                <span className="font-semibold">
                  {formatMoney(l.subtotalMinor, l.product.currency)}
                </span>
              </li>
            ))}
          </ul>
          <p className="flex justify-between text-xl">
            <span>Total</span>
            <span className="font-semibold">
              {formatMoney(cartTotalMinor(lines), SHOP_CURRENCY)}
            </span>
          </p>
          {delivery ? (
            <p className="text-text-secondary">
              Delivering to {delivery.address}, {delivery.city}. Phone:{" "}
              {delivery.phone}.
            </p>
          ) : null}
        </div>
      </Card>
      <Link href="/shop">Back to shop</Link>
    </Page>
  );
}
