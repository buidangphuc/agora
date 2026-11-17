import { Suspense } from "react";

import { redirectGuestToLogin } from "@/features/auth/guardPath";

import { Alert } from "@/components/ui/Alert";
import { GettingStartedCard } from "@/features/seller/GettingStartedCard";
import { KpiRow } from "@/features/seller/KpiRow";
import { LinkButton } from "@/features/seller/LinkButton";
import { LISTING_STATUSES, ProductList } from "@/features/seller/ProductList";
import { QuickActions } from "@/features/seller/QuickActions";
import { RECENT_ORDERS, RecentOrders } from "@/features/seller/RecentOrders";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { GettingStartedSkeleton } from "@/features/seller/SellerSkeletons";
import { deriveWorkplaceKpis } from "@/features/seller/kpi";
import {
  type SearchParams,
  buildListHref,
  parseListParams,
} from "@/features/seller/listParams";
import { LISTINGS_PAGE_SIZE } from "@/features/seller/listingsPage";
import { ListingStatus } from "@/generated/platform/listing/v1/listing_pb.js";
import { getStorefront, listMyListings } from "@/lib/gateway/listings";
import { type ViewOrder, listSellerOrdersPage } from "@/lib/gateway/orders";
import { listMyCampaigns, listMyShopVouchers } from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

const STATUS_FILTERS: Record<string, ListingStatus[]> = {
  published: [ListingStatus.PUBLISHED],
  draft: [ListingStatus.DRAFT],
  rejected: [ListingStatus.REJECTED],
};

function settled<T>(r: PromiseSettledResult<T>): T | null {
  return r.status === "fulfilled" ? r.value : null;
}

/** Dashboard > Workplace: KPIs, quick actions, recent orders, product Table List. */
export default async function SellerPage(props: {
  searchParams?: Promise<SearchParams>;
}) {
  const searchParams = await props.searchParams;
  const principal = getPrincipal();
  if (!principal) redirectGuestToLogin("/seller", searchParams);
  if (!hasScope("listing.write")) return null;

  const sellerId = principal?.id ?? "";
  const params = parseListParams(searchParams, LISTING_STATUSES);

  const statuses = STATUS_FILTERS[params.status] ?? [];
  const [pageResult, totalResult, publishedResult, ordersResult] =
    await Promise.allSettled([
      listMyListings({
        cursor: params.cursor,
        pageSize: LISTINGS_PAGE_SIZE,
        statuses,
        query: params.q,
      }),
      listMyListings({ pageSize: 1 }),
      listMyListings({ pageSize: 1, statuses: [ListingStatus.PUBLISHED] }),
      listSellerOrdersPage({ pageSize: RECENT_ORDERS }),
    ]);

  const listings = settled(pageResult);
  const orderPage = settled(ordersResult);
  const orders: ViewOrder[] | null = orderPage ? orderPage.orders : null;

  const kpis = deriveWorkplaceKpis({
    listingsTotal: settled(totalResult)?.total ?? null,
    publishedTotal: settled(publishedResult)?.total ?? null,
    statusCounts: orderPage ? orderPage.statusCounts : null,
  });
  const here = buildListHref("/seller", { q: params.q, status: params.status });

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

      <Suspense fallback={<GettingStartedSkeleton />}>
        <GettingStartedCard
          sources={{
            storefront: () => getStorefront(sellerId, { throwOnError: true }),
            listings: () => listMyListings({ pageSize: 1 }),
            vouchers: () => listMyShopVouchers({ pageSize: 1 }),
            campaigns: () => listMyCampaigns({ pageSize: 1 }),
          }}
        />
      </Suspense>

      {kpis.length > 0 && (
        <section aria-label="Số liệu tổng quan" className="space-y-3">
          <KpiRow
            cells={kpis.map((k) => ({
              key: k.key,
              title: k.title,
              value: k.value.toLocaleString("vi-VN"),
              note: k.note,
              tone: k.tone,
            }))}
          />
        </section>
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

      <div className="grid gap-6 lg:grid-cols-3 lg:items-start lg:gap-8">
        <div className="min-w-0 lg:col-span-2">
          <RecentOrders orders={orders} retryHref={here} />
        </div>
        <QuickActions />
      </div>
      <ProductList listings={listings} params={params} />
    </>
  );
}
