import { notFound } from "next/navigation";

import { OrderDetailView } from "@/features/seller/OrderDetailView";
import { PrintButton } from "@/features/seller/PrintButton";
import { PrintStyles } from "@/features/seller/PrintStyles";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { ShipOrderButton } from "@/features/seller/ShipOrderButton";
import { isShippable } from "@/features/seller/status";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { getShipmentTracking } from "@/lib/gateway/orders";
import { getPrincipal } from "@/lib/gateway/session";

import { isOwnedBy, loadSellerOrder } from "./data";

export const dynamic = "force-dynamic";

/** Profile > Advanced Profile on real order data. */
export default async function SellerOrderDetailPage({
  params,
  searchParams = {},
}: {
  params: { id: string };
  searchParams?: { tab?: string };
}) {
  const me = getPrincipal();
  const order = await loadSellerOrder(params.id);
  if (!order || !isOwnedBy(order, me)) {
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
