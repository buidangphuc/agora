import { Button } from "@fe-platform/ui";
import Link from "next/link";
import { notFound } from "next/navigation";
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
    <main className="flex flex-col gap-4 p-6">
      <h1 className="text-2xl">{product.title}</h1>
      <p className="text-text-secondary">
        {formatMoney(product.priceMinor, product.currency)}
      </p>
      <p>{product.description}</p>
      <form action="/shop/cart" method="get" className="flex gap-3">
        <input type="hidden" name="cart" value={next} />
        <Button type="submit" variant="primary">
          Add to cart
        </Button>
      </form>
      <Link href={`/shop${query}`}>Back to shop</Link>
      <Link href={`/shop/cart${query}`}>View cart</Link>
    </main>
  );
}
