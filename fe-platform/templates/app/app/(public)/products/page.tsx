import { Button, Card, Input, Link } from "@fe-platform/ui";
import { Page } from "../../../components/Page";
import { gatewayClients, loadProductPage } from "../../../lib/products";

export const dynamic = "force-dynamic";

export default async function ProductsPage({
  searchParams,
}: {
  searchParams: Promise<{ q?: string | string[]; cursor?: string }>;
}) {
  const sp = await searchParams;
  const q = (Array.isArray(sp.q) ? sp.q[0] : sp.q)?.trim() ?? "";
  const { items: products, nextCursor } = await loadProductPage(
    await gatewayClients(),
    { query: q, cursor: sp.cursor },
  );
  return (
    <Page title="Products" description={q ? `Results for "${q}"` : undefined}>
      <form
        action="/products"
        method="get"
        className="flex flex-col items-stretch gap-3 sm:flex-row sm:items-end"
      >
        <div className="flex-1">
          <Input label="Search products" name="q" defaultValue={q} />
        </div>
        <Button type="submit" variant="secondary">
          Search
        </Button>
      </form>
      {products.length === 0 ? (
        <p className="text-text-secondary">
          {q
            ? "Nothing matches that search."
            : "No products are published yet."}
        </p>
      ) : (
        <ul className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {products.map((p) => (
            <li key={p.id}>
              <Card>
                <div className="flex flex-col gap-4">
                  <h2 className="text-xl font-semibold">
                    <Link href={`/products/${p.id}`}>{p.title}</Link>
                  </h2>
                  <p className="text-text-secondary">{p.description}</p>
                  <p className="text-xl font-semibold">{p.price}</p>
                </div>
              </Card>
            </li>
          ))}
        </ul>
      )}
      {nextCursor ? (
        <Link
          href={`/products?${new URLSearchParams({ ...(q ? { q } : {}), cursor: nextCursor })}`}
        >
          Next page
        </Link>
      ) : null}
    </Page>
  );
}
