import Link from "next/link";
import { redirect } from "next/navigation";

import { Alert } from "@/components/ui/Alert";
import { Pagination } from "@/components/ui/Pagination";
import { RetryButton } from "@/features/account/RetryButton";
import { OrderList } from "@/features/order/OrderList";
import { OrderStatusTabs, ordersHref } from "@/features/order/OrderStatusTabs";
import { linkButton } from "@/features/order/linkStyles";
import {
  ORDERS_PAGE_SIZE,
  paginateOrders,
} from "@/features/order/paginateOrders";
import { listBuyerOrdersResult } from "@/lib/gateway/orders";
import { getPrincipal } from "@/lib/gateway/session";
import { batchGetShopNames } from "@/lib/gateway/shops";

export const dynamic = "force-dynamic";

export default async function AccountOrdersPage({
  searchParams,
}: {
  searchParams: { status?: string; page?: string };
}) {
  const me = getPrincipal();
  if (!me) redirect("/login");

  const result = await listBuyerOrdersResult();
  const view = paginateOrders(
    result.ok ? result.orders : [],
    searchParams.status,
    searchParams.page,
  );
  const shopNames = await batchGetShopNames(view.orders.map((o) => o.sellerId));

  return (
    <section className="mx-auto max-w-4xl space-y-4 py-2">
      <header>
        <h1 className="text-xl font-bold text-text-primary">
          Đơn hàng của tôi
        </h1>
        <p className="text-sm text-text-secondary">
          {result.ok
            ? `${view.counts.all} đơn hàng`
            : "Không tải được đơn hàng"}
        </p>
      </header>

      <OrderStatusTabs active={view.status} counts={view.counts} />

      {result.ok ? (
        <>
          <OrderList
            orders={view.orders}
            status={view.status}
            shopNames={shopNames}
          />
          <Pagination
            current={view.page}
            total={view.total}
            pageSize={ORDERS_PAGE_SIZE}
            hrefFor={(p) => ordersHref(view.status, p)}
            className="flex justify-center pt-2"
          />
        </>
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
