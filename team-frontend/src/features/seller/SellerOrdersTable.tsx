import { Link } from "@/components/ui/Link";
import { PriceTag } from "@/components/ui/PriceTag";
import { Table, type TableColumn } from "@/components/ui/Table";
import type { ViewOrder } from "@/lib/gateway/orders";
import { ShipOrderButton } from "./ShipOrderButton";
import { ShipByLabel } from "./shipBy";
import { OrderStatusTag, isShippable } from "./status";

function itemsSummary(o: ViewOrder): string {
  const first = o.items[0]?.title ?? "";
  if (o.items.length <= 1) return first;
  return `${first} +${o.items.length - 1} sản phẩm khác`;
}

const columns: TableColumn<ViewOrder>[] = [
  {
    key: "id",
    title: "Mã đơn",
    className: "sticky left-0 z-10 bg-surface-card md:static",
    render: (o) => (
      <div>
        <span className="font-medium text-text-primary">
          <Link href={`/seller/orders/${o.id}`} variant="subtle">
            #{o.id.slice(0, 8)}
          </Link>
        </span>
        <p className="text-xs text-text-disabled">{o.createdAt}</p>
        <p className="text-xs text-text-disabled">
          Hạn gửi: <ShipByLabel order={o} />
        </p>
      </div>
    ),
  },
  {
    key: "buyer",
    title: "Người nhận",
    render: (o) => (
      <div>
        <p>{o.recipientName}</p>
        <p className="text-xs text-text-disabled">{o.phone}</p>
      </div>
    ),
  },
  {
    key: "items",
    title: "Sản phẩm",
    className: "min-w-48",
    render: (o) => <span className="line-clamp-2">{itemsSummary(o)}</span>,
  },
  {
    key: "total",
    title: "Tổng tiền",
    render: (o) => <PriceTag price={o.totalAmount} size="md" />,
  },
  {
    key: "status",
    title: "Trạng thái",
    render: (o) => <OrderStatusTag status={o.status} text={o.statusText} />,
  },
  {
    key: "actions",
    title: "Thao tác",
    align: "right",
    render: (o) => (
      <div className="flex items-center justify-end gap-2">
        <Link href={`/seller/orders/${o.id}`} variant="ghost" size="xs">
          Chi tiết
        </Link>
        {isShippable(o.status) && <ShipOrderButton orderId={o.id} />}
      </div>
    ),
  },
];

/** Order Table List: server-rendered; only ShipOrderButton is a client island. */
export function SellerOrdersTable({ orders }: { orders: ViewOrder[] }) {
  return (
    <Table
      caption="Danh sách đơn hàng"
      columns={columns}
      dataSource={orders}
      rowKey="id"
    />
  );
}
