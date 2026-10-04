import Link from "next/link";

import { Alert } from "@/components/ui/Alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { PriceTag } from "@/components/ui/PriceTag";
import { Table, type TableColumn } from "@/components/ui/Table";
import type { ViewOrder } from "@/lib/gateway/orders";
import { LinkButton } from "./LinkButton";
import { OrderStatusTag } from "./status";

export const RECENT_ORDERS = 5;

const columns: TableColumn<ViewOrder>[] = [
  {
    key: "id",
    title: "Mã đơn",
    className: "sticky left-0 z-10 bg-surface-card md:static",
    render: (o) => (
      <Link
        href={`/seller/orders/${o.id}`}
        className="font-medium text-text-primary hover:text-action-primary"
      >
        #{o.id.slice(0, 8)}
      </Link>
    ),
  },
  { key: "buyer", title: "Người nhận", dataIndex: "recipientName" },
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
  { key: "createdAt", title: "Ngày đặt", dataIndex: "createdAt" },
];

/** Workplace "recent orders" block. `orders === null` = the call failed. */
export function RecentOrders({
  orders,
  retryHref,
}: { orders: ViewOrder[] | null; retryHref: string }) {
  let body: React.ReactNode;
  if (orders === null) {
    body = (
      <Alert
        type="error"
        description="Không tải được danh sách đơn hàng."
        action={
          <LinkButton href={retryHref} size="sm">
            Thử lại
          </LinkButton>
        }
      />
    );
  } else if (orders.length === 0) {
    body = (
      <Empty
        description="Chưa có đơn hàng nào"
        action={
          <LinkButton href="/seller/new" size="sm">
            Thêm sản phẩm
          </LinkButton>
        }
      />
    );
  } else {
    body = (
      <Table
        caption="Đơn hàng gần đây"
        columns={columns}
        dataSource={orders.slice(0, RECENT_ORDERS)}
        rowKey="id"
      />
    );
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>Đơn hàng gần đây</CardTitle>
        <Link
          href="/seller/orders"
          className="text-xs font-medium text-action-primary hover:underline"
        >
          Xem tất cả
        </Link>
      </CardHeader>
      <CardContent>{body}</CardContent>
    </Card>
  );
}
