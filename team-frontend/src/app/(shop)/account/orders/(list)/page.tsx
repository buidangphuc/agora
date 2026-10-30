import { redirectGuestToLogin } from "@/features/auth/guardPath";
import Link from "next/link";

import { Alert } from "@/components/ui/Alert";
import { Pagination } from "@/components/ui/Pagination";
import { RetryButton } from "@/features/account/RetryButton";
import { OrderList } from "@/features/order/OrderList";
import { OrderStatusTabs, ordersHref } from "@/features/order/OrderStatusTabs";
import {
  ORDERS_PAGE_SIZE,
  paginateOrders,
} from "@/features/order/paginateOrders";
import { listBuyerOrdersResult } from "@/lib/gateway/orders";
import { getPrincipal } from "@/lib/gateway/session";
import { batchGetShopNames } from "@/lib/gateway/shops";

export const dynamic = "force-dynamic";

export default async function AccountOrdersPage(props: {
  searchParams: Promise<{ status?: string; page?: string }>;
}) {
  const searchParams = await props.searchParams;
  const me = getPrincipal();
  if (!me) redirectGuestToLogin("/account/orders", searchParams);

  const result = await listBuyerOrdersResult();
  const view = paginateOrders(
    result.ok ? result.orders : [],
    searchParams.status,
    searchParams.page,
  );
  const shopNames = await batchGetShopNames(view.orders.map((o) => o.sellerId));

  return (
    <section className="mx-auto flex w-full max-w-4xl flex-col gap-6 py-4 md:gap-8 md:py-8">
      <header className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold tracking-tight text-text-primary md:text-3xl">
          Đơn hàng của tôi
        </h1>
        <p className="text-base text-text-secondary">
          {result.ok
            ? `${view.counts.all} đơn hàng`
            : "Không tải được đơn hàng"}
        </p>
      </header>

      <OrderStatusTabs active={view.status} counts={view.counts} />

      {result.ok ? (
        <div className="flex flex-col gap-6">
          <OrderList
            orders={view.orders}
            status={view.status}
            shopNames={shopNames}
          />
          <div className="flex justify-center">
            <Pagination
              current={view.page}
              total={view.total}
              pageSize={ORDERS_PAGE_SIZE}
              hrefFor={(p) => ordersHref(view.status, p)}
            />
          </div>
        </div>
      ) : (
        <Alert
          type="error"
          title="Không tải được danh sách đơn hàng"
          description="Đã có lỗi khi tải đơn hàng của bạn. Vui lòng thử lại."
          action={<RetryButton />}
        />
      )}
    </section>
  );
}
