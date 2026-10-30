import { Link, Table } from "@fe-platform/ui";
import { Page } from "../../../components/Page";
import { guarded } from "../../../lib/guard";
import { toOrderView } from "../../../lib/orders";

export const dynamic = "force-dynamic";

export default async function Dashboard() {
  const orders = (
    await guarded("/dashboard", (c) => c.order.listBuyerOrders({}))
  ).orders.map(toOrderView);
  return (
    <Page title="Your orders" description="Orders you have placed.">
      <Table
        caption="Your orders"
        columns={[
          { key: "order", header: "Order" },
          { key: "placed", header: "Placed" },
          { key: "status", header: "Status" },
          { key: "total", header: "Total" },
        ]}
        rows={orders.map((o) => ({
          id: o.id,
          order: (
            <Link href={`/shop/confirmation?order=${encodeURIComponent(o.id)}`}>
              {o.id}
            </Link>
          ),
          placed: o.placedOn,
          status: o.status,
          total: o.total,
        }))}
        emptyText="You have not placed any orders yet."
      />
    </Page>
  );
}
