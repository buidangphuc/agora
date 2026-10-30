import { Button, Card, Link } from "@fe-platform/ui";
import { notFound } from "next/navigation";
import { Page } from "../../../../components/Page";
import {
  addToCart,
  cartQuery,
  findShopProduct,
  formatMoney,
  parseCart,
  serializeCart,
} from "../../../../lib/shop";

export default async function ShopProductPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ cart?: string | string[] }>;
}) {
  const { id } = await params;
  const product = findShopProduct(id);
  if (!product) notFound();
  const { cart } = await searchParams;
  const current = parseCart(cart);
  const next = serializeCart(addToCart(current, product.id));
  const query = cartQuery(current);
  return (
    <Page title={product.title}>
      <div className="grid grid-cols-1 gap-8 md:grid-cols-2">
        <div
          aria-hidden="true"
          className="flex aspect-square items-center justify-center rounded-3xl border border-border-subtle bg-surface-muted text-4xl text-text-secondary"
        >
          {product.title.charAt(0)}
        </div>
        <div className="md:sticky md:top-6 md:self-start">
          <Card>
            <div className="flex flex-col gap-6">
              <p className="text-3xl font-semibold">
                {formatMoney(product.priceMinor, product.currency)}
              </p>
              <p className="text-text-secondary">{product.description}</p>
              <form
                action="/shop/cart"
                method="get"
                className="flex flex-col gap-3"
              >
                <input type="hidden" name="cart" value={next} />
                <Button type="submit" variant="primary" size="lg">
                  Add to cart
                </Button>
              </form>
              <div className="flex flex-col gap-2">
                <Link href={`/shop${query}`}>Back to shop</Link>
                <Link href={`/shop/cart${query}`}>View cart</Link>
              </div>
            </div>
          </Card>
        </div>
      </div>
    </Page>
  );
}
