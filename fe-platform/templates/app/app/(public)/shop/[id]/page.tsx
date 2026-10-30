import { redirect } from "next/navigation";

export default async function ShopProduct({
  params,
}: {
  params: Promise<{ id: string }>;
}): Promise<never> {
  redirect(`/products/${encodeURIComponent((await params).id)}`);
}
