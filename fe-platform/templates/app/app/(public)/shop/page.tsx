import { Card, Link } from "@fe-platform/ui";
import { Page } from "../../../components/Page";
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
    <Page title="Shop" description="Everyday things, simply made.">
      {SHOP_PRODUCTS.length === 0 ? (
        <p className="text-text-secondary">
          Nothing is for sale yet. Check back soon.
        </p>
      ) : (
        <ul className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {SHOP_PRODUCTS.map((p) => (
            <li key={p.id}>
              <Card>
                <div className="flex flex-col gap-4">
                  <h2 className="text-xl font-semibold">{p.title}</h2>
                  <p className="text-text-secondary">{p.description}</p>
                  <p className="text-xl font-semibold">
                    {formatMoney(p.priceMinor, p.currency)}
                  </p>
                  <Link href={`/shop/${p.id}${query}`}>View {p.title}</Link>
                </div>
              </Card>
            </li>
          ))}
        </ul>
      )}
      <Link href={`/shop/cart${query}`}>View cart</Link>
    </Page>
  );
}
