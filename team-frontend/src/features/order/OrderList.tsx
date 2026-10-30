import React from "react";

import { Card } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Link } from "@/components/ui/Link";
import { PriceTag } from "@/components/ui/PriceTag";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";
import { shopLabel } from "@/lib/gateway/shops";
import { ConfirmReceiptButton } from "./ConfirmReceiptButton";
import { OrderActions, ReviewButton } from "./OrderActions";
import { OrderItemThumb } from "./OrderItemThumb";
import { OrderStatusBadge } from "./OrderStatusBadge";
import { ordersHref } from "./OrderStatusTabs";
import { canPayAgain } from "./lifecycle";
import type { OrderStatusKey } from "./paginateOrders";

function formatInstant(iso: string): string {
  return new Date(iso).toLocaleString("vi-VN", {
    timeZone: "Asia/Ho_Chi_Minh",
  });
}

/** Cancel is offered on the list only while the order is still pending. */
export function canCancelFromList(status: OrderStatus): boolean {
  return status === OrderStatus.PENDING;
}

function OrderRow({
  order,
  shopName,
  first,
}: {
  order: ViewOrder;
  shopName?: string;
  first: boolean;
}) {
  const isOnlinePending =
    order.status === OrderStatus.PENDING &&
    order.paymentMethod !== PaymentMethod.COD &&
    order.paymentMethod !== PaymentMethod.UNSPECIFIED;

  const now = new Date();
  const payable = isOnlinePending && canPayAgain(order, now);
  const hasDue = isOnlinePending && order.paymentDueAt !== "";

  return (
    <Card data-testid="order-card">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border-subtle bg-surface-muted px-4 py-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold text-text-primary">
            {shopLabel(order.sellerId, shopName)}
          </p>
          <p className="text-xs text-text-secondary">
            Mã đơn: #{order.id.slice(0, 8)}
            {order.createdAt && <> · {order.createdAt}</>}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-text-secondary">
            {order.paymentMethodText}
          </span>
          <OrderStatusBadge
            status={order.status}
            label={order.statusText}
            data-testid="order-status"
          />
        </div>
      </div>

      <ul className="divide-y divide-border-subtle px-4">
        {order.items.map((it) => (
          <li key={it.id} className="flex items-start gap-3 py-3">
            <OrderItemThumb
              imageUrl={it.imageUrl}
              title={it.title}
              eager={first}
            />
            <div className="min-w-0 flex-1">
              <span className="line-clamp-2 text-sm font-medium text-text-primary">
                <Link href={`/listing/${it.listingId}`} variant="subtle">
                  {it.title}
                </Link>
              </span>
              {it.variantName && (
                <p className="text-xs text-text-secondary">
                  Phân loại: {it.variantName}
                </p>
              )}
              <p className="text-xs text-text-secondary">x{it.quantity}</p>
            </div>
            <div className="flex flex-col items-end gap-1.5">
              <PriceTag price={it.unitPrice * it.quantity} size="md" />
              {order.status === OrderStatus.COMPLETED && (
                <ReviewButton
                  listingId={it.listingId}
                  orderId={order.id}
                  productTitle={it.title}
                />
              )}
            </div>
          </li>
        ))}
      </ul>

      <div className="space-y-3 border-t border-border-subtle bg-surface-muted px-4 py-3">
        <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start">
          <div className="min-w-0 text-xs text-text-secondary">
            {order.recipientName && (
              <p>
                <span className="font-medium text-text-primary">
                  Giao đến:{" "}
                </span>
                {[order.recipientName, order.phone && `(${order.phone})`]
                  .filter(Boolean)
                  .join(" ")}
                {order.addressFull && ` - ${order.addressFull}`}
              </p>
            )}
            {hasDue && payable && (
              <p className="mt-0.5">
                Hạn thanh toán: {formatInstant(order.paymentDueAt)}
              </p>
            )}
            {hasDue && !payable && (
              <p className="mt-0.5">Đã hết hạn thanh toán</p>
            )}
            {order.trackingNumber && (
              <p className="mt-0.5">Mã vận đơn: {order.trackingNumber}</p>
            )}
          </div>
          <div className="flex shrink-0 items-baseline gap-2 sm:justify-end">
            <span className="text-xs text-text-secondary">
              Tổng thanh toán:
            </span>
            <PriceTag price={order.totalAmount} size="lg" />
          </div>
        </div>

        <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:justify-end">
          <Link
            href={`/account/orders/${order.id}`}
            variant="outline"
            size="xs"
            fullWidth="mobile"
          >
            Xem chi tiết
          </Link>
          {payable && (
            <Link
              href={`/checkout/pay/${order.id}`}
              variant="primary"
              size="xs"
              fullWidth="mobile"
            >
              Thanh toán lại
            </Link>
          )}
          {order.status === OrderStatus.SHIPPED && (
            <ConfirmReceiptButton orderId={order.id} />
          )}
          <OrderActions
            orderId={order.id}
            orderTotal={order.totalAmount}
            canCancel={canCancelFromList(order.status)}
            canReturn={false}
          />
        </div>
      </div>
    </Card>
  );
}

/** Empty state of the selected tab, with the way back. */
export function OrdersEmpty({ status }: { status: OrderStatusKey }) {
  if (status === "all") {
    return (
      <Empty
        description="Bạn chưa có đơn hàng nào"
        action={
          <Link href="/" variant="primary" size="xs">
            Mua sắm ngay
          </Link>
        }
      />
    );
  }
  return (
    <Empty
      description="Chưa có đơn hàng nào ở trạng thái này"
      action={
        <Link href={ordersHref("all")} variant="outline" size="xs">
          Xem tất cả đơn hàng
        </Link>
      }
    />
  );
}

/**
 * The buyer order list: one `Card` per order. Server component; only the
 * action buttons are client islands.
 */
export function OrderList({
  orders,
  status,
  shopNames,
}: {
  orders: ViewOrder[];
  status: OrderStatusKey;
  shopNames: Map<string, string>;
}) {
  if (orders.length === 0) return <OrdersEmpty status={status} />;
  return (
    <div className="space-y-4">
      {orders.map((o, i) => (
        <OrderRow
          key={o.id}
          order={o}
          shopName={shopNames.get(o.sellerId)}
          first={i === 0}
        />
      ))}
    </div>
  );
}
