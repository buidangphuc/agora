import { redirectGuestToLogin } from "@/features/auth/guardPath";

import { Alert } from "@/components/ui/Alert";
import { Card } from "@/components/ui/Card";
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
} from "@/features/seller/listParams";
import {
  ORDERS_PAGE_SIZE,
  ORDER_TABS,
  tabCountsFromStatusCounts,
  tabStatuses,
} from "@/features/seller/ordersList";
import {
  type SellerOrdersPage as OrdersPageData,
  listSellerOrdersPage,
} from "@/lib/gateway/orders";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Quản lý đơn hàng" };

const BASE = "/seller/orders";

/** List > Table List: status Tabs, search and page all live in the URL. */
export default async function SellerOrdersPage(props: {
  searchParams?: Promise<SearchParams>;
}) {
  const searchParams = await props.searchParams;
  const me = getPrincipal();
  if (!me) redirectGuestToLogin("/seller/orders", searchParams);
  if (!hasScope("listing.write")) return null;

  const params = parseListParams(searchParams, ORDER_TABS);

  let data: OrdersPageData | null = null;
  try {
    data = await listSellerOrdersPage({
      statuses: tabStatuses(params.status),
      query: params.q,
      cursor: params.cursor,
      pageSize: ORDERS_PAGE_SIZE,
    });
  } catch {
    data = null;
  }

  const here = buildListHref(BASE, { q: params.q, status: params.status });
  const counts = data ? tabCountsFromStatusCounts(data.statusCounts) : null;
  const rows = data?.orders ?? [];
  const filtering = params.q !== "" || params.status !== "all";

  let body: React.ReactNode;
  if (data === null) {
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
  } else if (rows.length === 0 && !filtering) {
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
        <div className="space-y-4 px-4 pt-4 sm:px-6 sm:pt-6">
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
            placeholder="Mã đơn hoặc tên sản phẩm"
          />
        </div>
        <div className="space-y-6 p-4 sm:p-6">
          {body}
          {data !== null && (
            <SellerPagination
              mode="cursor"
              current={params.page}
              firstHref={here}
              nextHref={
                data.nextCursor
                  ? buildListHref(BASE, {
                      q: params.q,
                      status: params.status,
                      cursor: data.nextCursor,
                      page: params.page + 1,
                    })
                  : null
              }
            />
          )}
        </div>
      </Card>
    </>
  );
}
