import { Link, Table } from "@fe-platform/ui";
import {
  SHOP_PRODUCTS,
  cartQuery,
  formatMoney,
  parseCart,
} from "../../../lib/shop";

export default async function ShopPage({
  searchParams,
}: {
  searchParams: Promise<{ cart?: string | string[] }>;
}) {
  const query = cartQuery(parseCart((await searchParams).cart));
  return (
    <main className="flex flex-col gap-4 p-6">
      <h1 className="text-2xl">Shop</h1>
      <Table
        caption="Products for sale"
        columns={[
          { key: "title", header: "Product" },
          { key: "description", header: "Description" },
          { key: "price", header: "Price" },
        ]}
        rows={SHOP_PRODUCTS.map((p) => ({
          id: p.id,
          title: <Link href={`/shop/${p.id}${query}`}>{p.title}</Link>,
          description: p.description,
          price: formatMoney(p.priceMinor, p.currency),
        }))}
        emptyText="Nothing is for sale yet. Check back soon."
      />
      <Link href={`/shop/cart${query}`}>View cart</Link>
    </main>
  );
}
