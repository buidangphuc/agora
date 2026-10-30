import { Alert } from "@/components/ui/Alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Link } from "@/components/ui/Link";
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
    sticky: true,
    render: (o) => (
      <span className="font-medium text-text-primary">
        <Link href={`/seller/orders/${o.id}`} variant="subtle">
          #{o.id.slice(0, 8)}
        </Link>
      </span>
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
        <Link href="/seller/orders" variant="ghost" size="xs">
          Xem tất cả
        </Link>
      </CardHeader>
      <CardContent>{body}</CardContent>
    </Card>
  );
}
