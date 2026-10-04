import { redirect } from "next/navigation";

import { Alert } from "@/components/ui/Alert";
import { Card, CardContent } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Tabs } from "@/components/ui/Tabs";
import { LinkButton } from "@/features/seller/LinkButton";
import { SellerFilterBar } from "@/features/seller/SellerFilterBar";
import { SellerOrdersTable } from "@/features/seller/SellerOrdersTable";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { SellerPagination } from "@/features/seller/SellerPagination";
import {
  type SearchParams,
  buildListHref,
  parseListParams,
  slicePage,
} from "@/features/seller/listParams";
import {
  ORDERS_PAGE_SIZE,
  ORDER_TABS,
  inTab,
  searchOrders,
  tabCounts,
} from "@/features/seller/ordersList";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { type ViewOrder, listSellerOrders } from "@/lib/gateway/orders";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Quản lý đơn hàng | Kênh người bán" };

const BASE = "/seller/orders";

/** List > Table List: status Tabs, search and page all live in the URL. */
export default async function SellerOrdersPage({
  searchParams = {},
}: { searchParams?: SearchParams }) {
  const me = getPrincipal();
  if (!me) redirect("/login");
  if (!hasScope("listing.write")) redirect("/");

  const params = parseListParams(searchParams, ORDER_TABS);

  let orders: ViewOrder[] | null = null;
  try {
    orders = await listSellerOrders(OrderStatus.UNSPECIFIED, {
      throwOnError: true,
    });
  } catch {
    orders = null;
  }

  const here = buildListHref(BASE, params);
  const counts = orders ? tabCounts(orders) : null;
  const inStatus = orders ? inTab(orders, params.status) : [];
  const matched = searchOrders(inStatus, params.q);
  const rows = slicePage(matched, params.page, ORDERS_PAGE_SIZE);
  const filtering = params.q !== "" || params.status !== "all";

  let body: React.ReactNode;
  if (orders === null) {
    body = (
      <Alert
        type="error"
        title="Không tải được danh sách đơn hàng"
        description="Có lỗi khi đọc dữ liệu từ máy chủ."
        action={
          <LinkButton href={here} size="sm">
            Thử lại
          </LinkButton>
        }
      />
    );
  } else if (orders.length === 0) {
    body = (
      <Empty description="Chưa có đơn hàng nào từ người mua. Khi có khách đặt mua, đơn sẽ hiển thị tại đây." />
    );
  } else if (rows.length === 0) {
    body = (
      <Empty
        description="Không có kết quả"
        action={
          filtering ? (
            <LinkButton href={BASE} size="sm">
              Xoá bộ lọc
            </LinkButton>
          ) : undefined
        }
      />
    );
  } else {
    body = <SellerOrdersTable orders={rows} />;
  }

  return (
    <>
      <SellerPageHeader
        title="Quản lý đơn hàng"
        description="Theo dõi, đóng gói và xác nhận gửi hàng tới người mua."
      />
      <Card>
        <CardContent className="space-y-4">
          <Tabs
            activeId={params.status}
            hrefFor={(id) => buildListHref(BASE, { q: params.q, status: id })}
            items={[
              { id: "all", label: "Tất cả", badge: counts?.all },
              { id: "pending", label: "Chờ xử lý", badge: counts?.pending },
              { id: "shipped", label: "Đang giao", badge: counts?.shipped },
              {
                id: "completed",
                label: "Hoàn thành",
                badge: counts?.completed,
              },
            ]}
          />
          <SellerFilterBar
            basePath={BASE}
            q={params.q}
            fixed={params.status === "all" ? {} : { status: params.status }}
            searchLabel="Tìm đơn hàng"
            placeholder="Mã đơn, người nhận, số điện thoại hoặc sản phẩm"
          />
          {body}
          {orders !== null && (
            <SellerPagination
              current={params.page}
              total={matched.length}
              pageSize={ORDERS_PAGE_SIZE}
              hrefFor={(page) => buildListHref(BASE, { ...params, page })}
            />
          )}
        </CardContent>
      </Card>
    </>
  );
}
