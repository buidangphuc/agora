import { Card, Link } from "@fe-platform/ui";
import { Page } from "../../../../components/Page";
import { guarded } from "../../../../lib/guard";
import { toOrderView } from "../../../../lib/orders";

export const dynamic = "force-dynamic";

const ID = /^[A-Za-z0-9_-]{1,64}$/;

export default async function ConfirmationPage({
  searchParams,
}: {
  searchParams: Promise<{ order?: string | string[] }>;
}) {
  const raw = (await searchParams).order;
  const ids = (Array.isArray(raw) ? raw[0] : raw)
    ?.split(",")
    .filter((id) => ID.test(id))
    .slice(0, 10);
  if (!ids || ids.length === 0) {
    return (
      <Page title="No order found" width="narrow">
        <p className="text-text-secondary">
          We could not find an order to show. Start from the catalogue.
        </p>
        <Link href="/products">Go to products</Link>
      </Page>
    );
  }
  const orders = (
    await guarded(`/shop/confirmation?order=${ids.join(",")}`, (c) =>
      Promise.all(ids.map((id) => c.order.getOrder({ id }))),
    )
  )
    .map((r) => r.order)
    .filter((o) => o !== undefined)
    .map(toOrderView);
  return (
    <Page
      title="Order placed"
      description={
        orders.length > 1
          ? `Your purchase was split into ${orders.length} orders, one per seller.`
          : "Thank you. We have your order."
      }
      width="narrow"
    >
      {orders.map((o) => (
        <Card key={o.id}>
          <div className="flex flex-col gap-6">
            <p className="flex justify-between gap-4">
              <span className="font-semibold">Order {o.id}</span>
              <span className="text-text-secondary">{o.status}</span>
            </p>
            <ul className="flex flex-col divide-y divide-border-subtle">
              {o.items.map((i) => (
                <li
                  key={`${i.title}-${i.variantName}`}
                  className="flex justify-between gap-4 py-3"
                >
                  <span>
                    {i.title}
                    {i.variantName ? ` (${i.variantName})` : ""} × {i.quantity}
                  </span>
                  <span className="font-semibold">{i.unitPrice} each</span>
                </li>
              ))}
            </ul>
            <p className="flex justify-between text-xl">
              <span>Total</span>
              <span className="font-semibold">{o.total}</span>
            </p>
            {o.address ? (
              <p className="text-text-secondary">
                Delivering to {o.recipient}, {o.address}.
              </p>
            ) : null}
          </div>
        </Card>
      ))}
      <Link href="/dashboard">View all orders</Link>
    </Page>
  );
}
