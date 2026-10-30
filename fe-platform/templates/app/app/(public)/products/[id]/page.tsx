import { Button, Card, Link, Select } from "@fe-platform/ui";
import { Page } from "../../../../components/Page";
import { errorMessage } from "../../../../lib/guard";
import { gatewayClients, loadProduct } from "../../../../lib/products";
import { addToCart } from "./actions";

export const dynamic = "force-dynamic";

export default async function ProductPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ error?: string }>;
}) {
  const { id } = await params;
  const message = errorMessage((await searchParams).error);
  const product = await loadProduct(await gatewayClients(), id);
  const hasVariants = product.variants.length > 0;
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
              <p className="text-3xl font-semibold">{product.price}</p>
              <p className="text-text-secondary">{product.description}</p>
              {message ? (
                <p role="alert" className="text-text-danger">
                  {message}
                </p>
              ) : null}
              <form action={addToCart} className="flex flex-col gap-4">
                <input type="hidden" name="listingId" value={product.id} />
                {hasVariants ? (
                  <Select
                    label="Option"
                    name="variantId"
                    defaultValue={product.variants[0]?.id}
                    options={product.variants.map((v) => ({
                      value: v.id,
                      label: `${v.name} (${v.price})`,
                    }))}
                  />
                ) : null}
                <Button type="submit" variant="primary" size="lg">
                  Add to cart
                </Button>
              </form>
              <Link href="/products">Back to products</Link>
            </div>
          </Card>
        </div>
      </div>
    </Page>
  );
}
