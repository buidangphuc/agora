import { Card, Link } from "@fe-platform/ui";
import { Page } from "../../../components/Page";
import { gatewayClients, loadProducts } from "../../../lib/products";

export const dynamic = "force-dynamic";

export default async function ProductsPage() {
  const products = await loadProducts(await gatewayClients());
  return (
    <Page title="Products" description="Published products.">
      {products.length === 0 ? (
        <p className="text-text-secondary">No products are published yet.</p>
      ) : (
        <ul className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {products.map((p) => (
            <li key={p.id}>
              <Card>
                <div className="flex flex-col gap-2">
                  <h2 className="text-xl font-semibold">
                    <Link href={`/products/${p.id}`}>{p.title}</Link>
                  </h2>
                  <p className="text-text-secondary">{p.price}</p>
                </div>
              </Card>
            </li>
          ))}
        </ul>
      )}
    </Page>
  );
}
