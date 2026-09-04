import { redirect } from "next/navigation";

import { BuyerOrdersList } from "@/features/order/BuyerOrdersList";
import { listBuyerOrders } from "@/lib/gateway/orders";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function AccountOrdersPage() {
  const me = getPrincipal();
  if (!me) redirect("/login");

  const orders = await listBuyerOrders();

  return (
    <section className="mx-auto max-w-3xl py-2">
      <BuyerOrdersList initialOrders={orders} />
    </section>
  );
}
