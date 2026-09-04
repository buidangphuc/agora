import { notFound, redirect } from "next/navigation";

import { OrderDetailView } from "@/features/order/OrderDetailView";
import { OrderTimeline } from "@/features/order/OrderTimeline";
import { ReturnRequestSection } from "@/features/order/ReturnRequestSection";
import {
  getOrder,
  getSagaState,
  getShipmentTracking,
} from "@/lib/gateway/orders";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function BuyerOrderDetailPage({
  params,
}: {
  params: { id: string };
}) {
  const me = getPrincipal();
  if (!me) redirect("/login");

  const order = await getOrder(params.id);
  if (!order) notFound();

  const [shipment, sagaSteps] = await Promise.all([
    getShipmentTracking(order.id),
    getSagaState(order.id),
  ]);

  return (
    <section className="py-2">
      <OrderDetailView order={order} />
      <OrderTimeline shipment={shipment} sagaSteps={sagaSteps} />
      <ReturnRequestSection orderId={order.id} orderTotal={order.totalAmount} />
    </section>
  );
}
