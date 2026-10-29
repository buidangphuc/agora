import { Table } from "@fe-platform/ui";
import Link from "next/link";
import {
  SHOP_CURRENCY,
  cartQuery,
  cartTotalMinor,
  formatMoney,
  parseCart,
} from "../../../../lib/shop";

export default async function CartPage({
  searchParams,
}: {
  searchParams: Promise<{ cart?: string | string[] }>;
}) {
  const lines = parseCart((await searchParams).cart);
  const query = cartQuery(lines);
  return (
    <main className="flex flex-col gap-4 p-6">
      <h1 className="text-2xl">Cart</h1>
      <Table
        caption="Items in your cart"
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
        emptyText="Your cart is empty. Pick something from the shop."
      />
      <p className="text-xl">
        Total: {formatMoney(cartTotalMinor(lines), SHOP_CURRENCY)}
      </p>
      <Link href={`/shop${query}`}>Continue shopping</Link>
      {lines.length > 0 ? (
        <Link href={`/shop/checkout${query}`}>Continue to checkout</Link>
      ) : null}
    </main>
  );
}
