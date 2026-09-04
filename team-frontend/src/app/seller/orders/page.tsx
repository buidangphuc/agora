import { redirect } from "next/navigation";

import { SellerOrdersList } from "@/features/order/SellerOrdersList";
import { listSellerOrders } from "@/lib/gateway/orders";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function SellerOrdersPage() {
  const me = getPrincipal();
  if (!me) redirect("/login");
  if (!hasScope("listing.write")) redirect("/");

  const orders = await listSellerOrders();

  return (
    <section className="mx-auto max-w-3xl py-2">
      <SellerOrdersList initialOrders={orders} />
    </section>
  );
}
