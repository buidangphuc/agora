import { notFound } from "next/navigation";

import { OrderDetailView } from "@/features/seller/OrderDetailView";
import { PrintButton } from "@/features/seller/PrintButton";
import { PrintStyles } from "@/features/seller/PrintStyles";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { ShipOrderButton } from "@/features/seller/ShipOrderButton";
import { isShippable } from "@/features/seller/status";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import {
  type ViewOrder,
  getOrder,
  getShipmentTracking,
  listSellerOrders,
} from "@/lib/gateway/orders";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

/**
 * Seller view of one order: getOrder first; if the gateway does not let a
 * seller principal read it, fall back to the seller's own order list.
 */
async function loadOrder(id: string): Promise<ViewOrder | null> {
  const direct = await getOrder(id);
  if (direct) return direct;
  const mine = await listSellerOrders(OrderStatus.UNSPECIFIED);
  return mine.find((o) => o.id === id) ?? null;
}

/** Profile > Advanced Profile on real order data. */
export default async function SellerOrderDetailPage({
  params,
  searchParams = {},
}: {
  params: { id: string };
  searchParams?: { tab?: string };
}) {
  const me = getPrincipal();
  const order = await loadOrder(params.id);
  const owned =
    order !== null &&
    me !== null &&
    (order.sellerId === me.id || me.scopes.includes("admin"));
  if (!order || !owned) {
    notFound();
    return null;
  }

  const shipment = await getShipmentTracking(order.id);
  const tab = searchParams.tab === "shipment" ? "shipment" : "items";
  const base = `/seller/orders/${order.id}`;

  return (
    <>
      <PrintStyles />
      <SellerPageHeader
        title={`Đơn hàng #${order.id.slice(0, 8)}`}
        description="Xử lý đơn hàng và in phiếu đóng gói."
        trail={[{ label: "Quản lý đơn hàng", href: "/seller/orders" }]}
        action={
          <div className="flex flex-wrap items-center gap-3">
            <PrintButton />
            {isShippable(order.status) && (
              <ShipOrderButton
                orderId={order.id}
                label="Bàn giao vận chuyển"
                variant="primary"
                size="md"
                withConfirm
              />
            )}
          </div>
        }
      />
      <OrderDetailView
        order={order}
        shipment={shipment}
        tab={tab}
        tabHref={(id) => (id === "items" ? base : `${base}?tab=${id}`)}
      />
    </>
  );
}
