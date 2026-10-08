import Link from "next/link";
import { redirect } from "next/navigation";
import { Suspense } from "react";

import { Alert } from "@/components/ui/Alert";
import { Result } from "@/components/ui/Result";
import { OrderDetailView } from "@/features/order/OrderDetailView";
import {
  OrderTimelineSection,
  OrderTimelineSkeleton,
} from "@/features/order/OrderTimelineSection";
import { ReturnStateProvider } from "@/features/order/ReturnState";
import { parseDetailTab } from "@/features/order/detailTab";
import { linkButton } from "@/features/order/linkStyles";
import { listOrderReturns } from "@/lib/gateway/orders";
import { getPrincipal } from "@/lib/gateway/session";

import { loadOrderResult } from "./data";

export const dynamic = "force-dynamic";

const backLink = (
  <Link href="/account/orders" className={linkButton.outlineMd}>
    Về đơn hàng của tôi
  </Link>
);

export default async function BuyerOrderDetailPage({
  params,
  searchParams,
}: {
  params: { id: string };
  searchParams: { tab?: string };
}) {
  const me = getPrincipal();
  if (!me) redirect("/login");

  // Resolve ownership first: nothing else (shipment, saga, items) is fetched
  // or rendered unless the order is the caller's own.
  const res = await loadOrderResult(params.id);

  if (res.kind === "forbidden") {
    return (
      <Result
        status="403"
        title="Bạn không có quyền xem đơn hàng này"
        subTitle="Đơn hàng này thuộc về một tài khoản khác."
        extra={backLink}
      />
    );
  }
  if (res.kind === "not_found") {
    return (
      <Result
        status="404"
        title="Không tìm thấy đơn hàng"
        subTitle="Đơn hàng không tồn tại hoặc đã bị xóa."
        extra={backLink}
      />
    );
  }
  if (res.kind === "error") {
    return (
      <section className="mx-auto max-w-4xl py-2">
        <Alert
          type="error"
          title="Không tải được đơn hàng"
          description="Đã có lỗi khi tải đơn hàng này. Vui lòng thử lại."
          action={
            <Link
              href={`/account/orders/${params.id}`}
              className={linkButton.outline}
            >
              Thử lại
            </Link>
          }
        />
      </section>
    );
  }

  // Server data, so the returns are still listed after a reload.
  const returns = await listOrderReturns(res.order.id);

  return (
    <section className="py-2">
      <ReturnStateProvider initialReturns={returns}>
        <OrderDetailView
          order={res.order}
          tab={parseDetailTab(searchParams.tab)}
          timeline={
            <Suspense fallback={<OrderTimelineSkeleton />}>
              <OrderTimelineSection orderId={res.order.id} />
            </Suspense>
          }
        />
      </ReturnStateProvider>
    </section>
  );
}
