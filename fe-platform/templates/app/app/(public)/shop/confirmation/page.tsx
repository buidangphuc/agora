import { Table } from "@fe-platform/ui";
import Link from "next/link";
import {
  SHOP_CURRENCY,
  cartTotalMinor,
  formatMoney,
  parseCart,
  textParam,
} from "../../../../lib/shop";

type Search = Record<string, string | string[] | undefined>;

export default async function ConfirmationPage({
  searchParams,
}: {
  searchParams: Promise<Search>;
}) {
  const sp = await searchParams;
  const lines = parseCart(sp.cart);
  if (lines.length === 0) {
    return (
      <main className="flex flex-col gap-4 p-6">
        <h1 className="text-2xl">No order found</h1>
        <p>We could not find an order to show. Start from the shop.</p>
        <Link href="/shop">Go to shop</Link>
      </main>
    );
  }
  return (
    <main className="flex flex-col gap-4 p-6">
      <h1 className="text-2xl">Order placed</h1>
      <p className="text-text-secondary">
        Thank you, {textParam(sp.name)}. Your order is on its way.
      </p>
      <Table
        caption="Order summary"
        columns={[
          { key: "title", header: "Product" },
          { key: "quantity", header: "Quantity" },
          { key: "subtotal", header: "Subtotal" },
        ]}
        rows={lines.map((l) => ({
          id: l.product.id,
          title: l.product.title,
          quantity: l.quantity,
          subtotal: formatMoney(l.subtotalMinor, l.product.currency),
        }))}
      />
      <p className="text-xl">
        Total: {formatMoney(cartTotalMinor(lines), SHOP_CURRENCY)}
      </p>
      <p>
        Delivering to {textParam(sp.address)}, {textParam(sp.city)}. Phone:{" "}
        {textParam(sp.phone)}.
      </p>
      <Link href="/shop">Back to shop</Link>
    </main>
  );
}
