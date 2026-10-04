import Link from "next/link";
import React from "react";

import { Alert } from "@/components/ui/Alert";
import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { Descriptions } from "@/components/ui/Descriptions";
import { PriceTag } from "@/components/ui/PriceTag";
import { type StepItem, Stepper } from "@/components/ui/Stepper";
import { Table, type TableColumn } from "@/components/ui/Table";
import { formatPrice } from "@/components/ui/format";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewOrder, ViewOrderItem } from "@/lib/gateway/orders";
import { OrderActions, ReviewButton } from "./OrderActions";
import { OrderDetailTabs } from "./OrderDetailTabs";
import { OrderItemThumb } from "./OrderItemThumb";
import { OrderStatusBadge } from "./OrderStatusBadge";
import { ReturnRequestSection } from "./ReturnRequestSection";
import type { OrderDetailTab } from "./detailTab";

/** Cancel on the detail is offered until the order is completed or cancelled. */
export function canCancelFromDetail(status: OrderStatus): boolean {
  return (
    status === OrderStatus.PENDING ||
    status === OrderStatus.PAID ||
    status === OrderStatus.SHIPPED
  );
}

/** Returns are offered for completed orders only. */
export function canReturnOrder(status: OrderStatus): boolean {
  return status === OrderStatus.COMPLETED;
}

const STEP_TITLES = [
  "Đã đặt hàng",
  "Đã thanh toán",
  "Đóng gói",
  "Đang vận chuyển",
  "Đã nhận hàng",
];

/** The step the order is on (1-based); a completed order has finished all five. */
function currentStep(status: OrderStatus): number {
  switch (status) {
    case OrderStatus.COMPLETED:
      return 6;
    case OrderStatus.SHIPPED:
      return 4;
    case OrderStatus.PAID:
      return 3;
    case OrderStatus.PENDING:
      return 2;
    default:
      return 1;
  }
}

export function orderSteps(status: OrderStatus): StepItem[] {
  const current = currentStep(status);
  return STEP_TITLES.map((title, i) => {
    const n = i + 1;
    return {
      id: n,
      title,
      status: n < current ? "complete" : n === current ? "current" : "upcoming",
    };
  });
}

function itemColumns(order: ViewOrder): TableColumn<ViewOrderItem>[] {
  return [
    {
      key: "product",
      title: "Sản phẩm",
      render: (it, index) => (
        <div className="flex items-start gap-3">
          <OrderItemThumb
            imageUrl={it.imageUrl}
            title={it.title}
            eager={index === 0}
          />
          <div className="min-w-0">
            <Link
              href={`/listing/${it.listingId}`}
              className="line-clamp-2 text-sm font-medium text-text-primary hover:text-action-primary"
            >
              {it.title}
            </Link>
            {it.variantName && (
              <p className="text-xs text-text-secondary">
                Phân loại: {it.variantName}
              </p>
            )}
            <div className="flex items-center gap-1 text-xs text-text-secondary md:hidden">
              {it.quantity} × <PriceTag price={it.unitPrice} size="sm" />
            </div>
            {order.status === OrderStatus.COMPLETED && (
              <div className="mt-1.5">
                <ReviewButton
                  listingId={it.listingId}
                  orderId={order.id}
                  productTitle={it.title}
                />
              </div>
            )}
          </div>
        </div>
      ),
    },
    {
      key: "price",
      title: "Đơn giá",
      align: "right",
      className: "hidden md:table-cell",
      render: (it) => <PriceTag price={it.unitPrice} size="sm" />,
    },
    {
      key: "qty",
      title: "Số lượng",
      align: "center",
      className: "hidden md:table-cell",
      render: (it) => it.quantity,
    },
    {
      key: "subtotal",
      title: "Thành tiền",
      align: "right",
      render: (it) => <PriceTag price={it.unitPrice * it.quantity} size="md" />,
    },
  ];
}

/**
 * The order detail (Ant Design Pro "Advanced Profile"): header, progress band,
 * description blocks, items table and the Hành trình / Trả hàng tabs.
 * A server component; actions and tabs are client islands.
 */
export function OrderDetailView({
  order,
  timeline,
  tab = "timeline",
}: {
  order: ViewOrder;
  /** The (Suspense-wrapped) order timeline. */
  timeline: React.ReactNode;
  tab?: OrderDetailTab;
}) {
  const cancelled = order.status === OrderStatus.CANCELLED;
  const steps = orderSteps(order.status);

  const recipient = [
    { key: "recipient", label: "Người nhận", children: order.recipientName },
    { key: "phone", label: "Số điện thoại", children: order.phone },
    { key: "address", label: "Địa chỉ nhận hàng", children: order.addressFull },
    {
      key: "payment",
      label: "Hình thức thanh toán",
      children: order.paymentMethodText,
    },
    ...(order.voucherCode
      ? [{ key: "voucher", label: "Mã giảm giá", children: order.voucherCode }]
      : []),
    ...(order.trackingNumber
      ? [
          {
            key: "tracking",
            label: "Mã vận đơn",
            children: order.trackingNumber,
          },
        ]
      : []),
  ].filter((d) => d.children !== "");

  const amounts = [
    {
      key: "subtotal",
      label: "Tổng tiền hàng",
      children: <PriceTag price={order.itemsSubtotal} size="md" />,
    },
    {
      key: "shipping",
      label: "Phí vận chuyển",
      children: <PriceTag price={order.shippingFee} size="md" />,
    },
    ...(order.discountAmount > 0
      ? [
          {
            key: "discount",
            label: "Giảm giá",
            children: (
              <span className="text-accent-success-dark">
                -{formatPrice(order.discountAmount)}
              </span>
            ),
          },
        ]
      : []),
    {
      key: "total",
      label: "Tổng thanh toán",
      children: <PriceTag price={order.totalAmount} size="lg" />,
    },
  ];

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <header className="space-y-4 rounded-xl border border-border-subtle bg-surface-card p-5">
        <Breadcrumb
          items={[
            { label: "Đơn hàng của tôi", href: "/account/orders" },
            { label: `Chi tiết #${order.id.slice(0, 8)}` },
          ]}
        />
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0 space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-2xl font-bold text-text-primary">
                Order #{order.id.slice(0, 8)}
              </h1>
              <OrderStatusBadge
                status={order.status}
                label={order.statusText}
                data-testid="order-status"
              />
            </div>
            {order.createdAt && (
              <p className="text-xs text-text-secondary">
                Ngày đặt: {order.createdAt}
              </p>
            )}
            <div className="pt-1">
              <PriceTag price={order.totalAmount} size="xl" />
            </div>
          </div>
          <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap lg:justify-end">
            <OrderActions
              orderId={order.id}
              orderTotal={order.totalAmount}
              primaryReorder
              canCancel={canCancelFromDetail(order.status)}
              canReturn={canReturnOrder(order.status)}
            />
          </div>
        </div>
      </header>

      <section aria-label="Tiến trình đơn hàng">
        {cancelled ? (
          <Alert
            type="warning"
            title="Đơn hàng đã hủy"
            description="Tồn kho sản phẩm đã được hoàn lại cho người bán."
          />
        ) : (
          <div className="rounded-xl border border-border-subtle bg-surface-card p-5">
            <Stepper steps={steps} className="hidden sm:block" />
            <Stepper
              steps={steps}
              orientation="vertical"
              className="sm:hidden"
            />
          </div>
        )}
      </section>

      <Descriptions title="Thông tin đơn hàng" items={recipient} />

      <section aria-label="Sản phẩm" className="space-y-3">
        <h2 className="text-sm font-bold text-text-primary">
          Sản phẩm ({order.items.length})
        </h2>
        <Table
          columns={itemColumns(order)}
          dataSource={order.items}
          rowKey="id"
          caption="Danh sách sản phẩm của đơn hàng"
        />
        <Descriptions items={amounts} column={1} />
      </section>

      <OrderDetailTabs
        initialTab={tab}
        timeline={timeline}
        returns={
          <ReturnRequestSection
            orderId={order.id}
            orderTotal={order.totalAmount}
            canRequest={canReturnOrder(order.status)}
          />
        }
      />
    </div>
  );
}
