import { Link } from "@fe-platform/ui";
import { gatewayClients, loadProduct } from "../../../../lib/products";

export const dynamic = "force-dynamic";

export default async function ProductPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const product = await loadProduct(await gatewayClients(), id);
  return (
    <main className="flex flex-col gap-4 p-6">
      <h1 className="text-2xl">{product.title}</h1>
      <p className="text-text-secondary">{product.price}</p>
      <p>{product.description}</p>
      <Link href="/products">Back to products</Link>
    </main>
  );
}
