import { Table } from "@fe-platform/ui";
import Link from "next/link";
import { gatewayClients, loadProducts } from "../../../lib/products";

export const dynamic = "force-dynamic";

export default async function ProductsPage() {
  const products = await loadProducts(await gatewayClients());
  return (
    <main className="flex flex-col gap-4 p-6">
      <h1 className="text-2xl">Products</h1>
      <Table
        caption="Published products"
        columns={[
          { key: "title", header: "Title" },
          { key: "price", header: "Price" },
        ]}
        rows={products.map((p) => ({
          id: p.id,
          title: <Link href={`/products/${p.id}`}>{p.title}</Link>,
          price: p.price,
        }))}
        emptyText="No products are published yet."
      />
    </main>
  );
}
