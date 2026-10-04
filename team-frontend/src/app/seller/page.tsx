import { redirect } from "next/navigation";

import { Alert } from "@/components/ui/Alert";
import { KpiRow } from "@/features/seller/KpiRow";
import { LinkButton } from "@/features/seller/LinkButton";
import { LISTING_STATUSES, ProductList } from "@/features/seller/ProductList";
import { QuickActions } from "@/features/seller/QuickActions";
import { RecentOrders } from "@/features/seller/RecentOrders";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { deriveWorkplaceKpis } from "@/features/seller/kpi";
import {
  type SearchParams,
  buildListHref,
  parseListParams,
} from "@/features/seller/listParams";
import {
  LISTINGS_PAGE_SIZE,
  getListingsPage,
} from "@/features/seller/listingsPage";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { type ListingPage, listMyListings } from "@/lib/gateway/listings";
import { type ViewOrder, listSellerOrders } from "@/lib/gateway/orders";
import { hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

function settled<T>(r: PromiseSettledResult<T>): T | null {
  return r.status === "fulfilled" ? r.value : null;
}

/** Dashboard > Workplace: KPIs, quick actions, recent orders, product Table List. */
export default async function SellerPage({
  searchParams = {},
}: { searchParams?: SearchParams }) {
  if (!hasScope("listing.write")) redirect("/login");

  const params = parseListParams(searchParams, LISTING_STATUSES);

  const pageRequest = getListingsPage(params.page);
  // KPIs always describe the first page of products, whatever page is shown.
  const kpiRequest: Promise<ListingPage> =
    params.page === 1 ? pageRequest : listMyListings();
  const [pageResult, kpiResult, ordersResult] = await Promise.allSettled([
    pageRequest,
    kpiRequest,
    listSellerOrders(OrderStatus.UNSPECIFIED, { throwOnError: true }),
  ]);

  const listings = settled(pageResult);
  const kpiListings = settled(kpiResult);
  const orders: ViewOrder[] | null = settled(ordersResult);

  const kpis = deriveWorkplaceKpis({ listings: kpiListings, orders });
  const partial =
    kpiListings !== null && kpiListings.total > kpiListings.items.length;
  const here = buildListHref("/seller", params);

  return (
    <>
      <SellerPageHeader
        title="Tổng quan gian hàng"
        description="Theo dõi sản phẩm, đơn hàng và thao tác nhanh của shop."
        action={
          <LinkButton href="/seller/new" variant="primary">
            Thêm sản phẩm
          </LinkButton>
        }
      />

      {kpis.length > 0 && (
        <div className="space-y-2">
          <KpiRow
            cells={kpis.map((k) => ({
              key: k.key,
              title: k.title,
              value: k.value.toLocaleString("vi-VN"),
            }))}
          />
          {partial && (
            <p className="text-xs text-text-secondary">
              "Đang bán" và "Sắp hết hàng" tính trên {LISTINGS_PAGE_SIZE} sản
              phẩm đầu tiên.
            </p>
          )}
        </div>
      )}
      {kpis.length === 0 && (
        <Alert
          type="warning"
          description="Không tải được số liệu tổng quan."
          action={
            <LinkButton href={here} size="sm">
              Thử lại
            </LinkButton>
          }
        />
      )}

      <QuickActions />
      <RecentOrders orders={orders} retryHref={here} />
      <ProductList listings={listings} params={params} />
    </>
  );
}
