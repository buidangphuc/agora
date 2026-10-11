import React from "react";

import { Alert } from "@/components/ui/Alert";
import { Card, CardContent } from "@/components/ui/Card";
import { Descriptions } from "@/components/ui/Descriptions";
import { Empty } from "@/components/ui/Empty";
import { Image } from "@/components/ui/Image";
import { PriceTag } from "@/components/ui/PriceTag";
import { Statistic } from "@/components/ui/Statistic";
import { type StepItem, Stepper } from "@/components/ui/Stepper";
import { Table, type TableColumn } from "@/components/ui/Table";
import { Tabs } from "@/components/ui/Tabs";
import { Timeline } from "@/components/ui/Timeline";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import type {
  ViewOrder,
  ViewOrderItem,
  ViewShipment,
} from "@/lib/gateway/orders";
import { getImageUrl } from "@/lib/media";
import { OrderStatusTag } from "./status";

const FLOW: { status: OrderStatus; title: string }[] = [
  { status: OrderStatus.PENDING, title: "Chờ xử lý" },
  { status: OrderStatus.PAID, title: "Đã thanh toán" },
  { status: OrderStatus.SHIPPED, title: "Đang giao" },
  { status: OrderStatus.COMPLETED, title: "Hoàn thành" },
];

/** Fulfilment steps for the Stepper; `aria-current="step"` marks the current one. */
export function fulfilmentSteps(status: OrderStatus): StepItem[] {
  const at = FLOW.findIndex((s) => s.status === status);
  return FLOW.map((s, i) => ({
    id: s.status,
    title: s.title,
    status:
      status === OrderStatus.COMPLETED || i < at
        ? "complete"
        : i === at
          ? "current"
          : "upcoming",
  }));
}

const itemColumns: TableColumn<ViewOrderItem>[] = [
  {
    key: "product",
    title: "Sản phẩm",
    className: "min-w-56",
    render: (it) => (
      <div className="flex items-center gap-3">
        <div className="w-12 shrink-0">
          <Image
            src={it.imageUrl ? getImageUrl(it.imageUrl) : ""}
            alt={it.title}
            aspect="square"
            className="rounded-lg"
          />
        </div>
        <span className="line-clamp-2 font-medium">{it.title}</span>
      </div>
    ),
  },
  { key: "variant", title: "Phân loại", render: (it) => it.variantName || "—" },
  { key: "qty", title: "SL", align: "center", render: (it) => it.quantity },
  {
    key: "unit",
    title: "Đơn giá",
    align: "right",
    render: (it) => <PriceTag price={it.unitPrice} size="sm" />,
  },
  {
    key: "line",
    title: "Thành tiền",
    align: "right",
    render: (it) => <PriceTag price={it.unitPrice * it.quantity} size="md" />,
  },
];

function ItemsPanel({ order }: { order: ViewOrder }) {
  return (
    <div className="space-y-4">
      <Table
        caption="Hàng hoá trong đơn"
        columns={itemColumns}
        dataSource={order.items}
        rowKey="id"
      />
      <Descriptions
        column={2}
        items={[
          {
            key: "subtotal",
            label: "Tiền hàng",
            children: <PriceTag price={order.itemsSubtotal} size="sm" />,
          },
          {
            key: "ship",
            label: "Phí vận chuyển",
            children: <PriceTag price={order.shippingFee} size="sm" />,
          },
          ...(order.discountAmount > 0
            ? [
                {
                  key: "discount",
                  label: order.voucherCode
                    ? `Giảm giá (${order.voucherCode})`
                    : "Giảm giá",
                  children: <PriceTag price={order.discountAmount} size="sm" />,
                },
              ]
            : []),
          {
            key: "total",
            label: "Tổng thanh toán",
            children: <PriceTag price={order.totalAmount} size="md" />,
          },
        ]}
      />
    </div>
  );
}

function ShipmentPanel({ shipment }: { shipment: ViewShipment | null }) {
  if (!shipment) {
    return <Empty description="Chưa có thông tin vận chuyển" />;
  }
  return (
    <div className="space-y-4">
      <Descriptions
        column={3}
        items={[
          {
            key: "carrier",
            label: "Đơn vị vận chuyển",
            children: shipment.carrier || "—",
          },
          {
            key: "code",
            label: "Mã vận đơn",
            children: shipment.trackingCode || "—",
          },
          { key: "status", label: "Trạng thái", children: shipment.statusText },
        ]}
      />
      <Timeline
        emptyText="Chưa có điểm theo dõi"
        items={shipment.checkpoints.map((c, i, all) => ({
          key: `${i}-${c.timestamp}`,
          title: c.description || c.location,
          description: c.description ? c.location : undefined,
          time: c.timestamp,
          current: i === all.length - 1,
          tone: i === all.length - 1 ? "primary" : "neutral",
        }))}
      />
    </div>
  );
}

/** Advanced Profile body of /seller/orders/[id]: everything inside is the packing slip. */
export function OrderDetailView({
  order,
  shipment,
  tab,
  tabHref,
  returnsPanel,
}: {
  order: ViewOrder;
  shipment: ViewShipment | null;
  tab: "items" | "shipment" | "returns";
  tabHref: (tab: string) => string;
  /** The returns tab body (server-composed: returns and payment are fetched by the page). */
  returnsPanel?: React.ReactNode;
}) {
  const cancelled = order.status === OrderStatus.CANCELLED;
  return (
    <section data-testid="packing-slip" className="space-y-6">
      <Card>
        <CardContent className="grid gap-4 md:grid-cols-3">
          <div className="md:col-span-2">
            <Descriptions
              title={`Đơn hàng #${order.id.slice(0, 8)}`}
              column={2}
              items={[
                { key: "id", label: "Mã đơn", children: order.id },
                {
                  key: "created",
                  label: "Ngày tạo",
                  children: order.createdAt || "—",
                },
                {
                  key: "payment",
                  label: "Thanh toán",
                  children: order.paymentMethodText,
                },
                {
                  key: "tracking",
                  label: "Mã vận đơn",
                  children: order.trackingNumber || "—",
                },
              ]}
            />
          </div>
          <Statistic
            title="Trạng thái"
            value={
              <OrderStatusTag status={order.status} text={order.statusText} />
            }
          />
        </CardContent>
      </Card>

      {cancelled ? (
        <Alert type="info" description="Đơn hàng này đã bị hủy." />
      ) : (
        <Card>
          <CardContent>
            <Stepper steps={fulfilmentSteps(order.status)} />
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent className="space-y-4">
          <div data-print-hidden>
            <Tabs
              activeId={tab}
              hrefFor={tabHref}
              items={[
                { id: "items", label: "Hàng hoá", badge: order.items.length },
                { id: "shipment", label: "Vận chuyển" },
                { id: "returns", label: "Trả hàng / Hoàn tiền" },
              ]}
            />
          </div>
          {tab === "shipment" ? (
            <ShipmentPanel shipment={shipment} />
          ) : tab === "returns" ? (
            returnsPanel
          ) : (
            <ItemsPanel order={order} />
          )}
        </CardContent>
      </Card>

      <Card>
        <CardContent>
          <Descriptions
            title="Người nhận"
            column={1}
            items={[
              {
                key: "name",
                label: "Họ tên",
                children: order.recipientName || "—",
              },
              {
                key: "phone",
                label: "Số điện thoại",
                children: order.phone || "—",
              },
              {
                key: "address",
                label: "Địa chỉ giao hàng",
                children: order.addressFull || "—",
              },
            ]}
          />
        </CardContent>
      </Card>
    </section>
  );
}
