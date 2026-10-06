import {
  FormCardSkeleton,
  HeaderSkeleton,
  KpiRowSkeleton,
  TableCardSkeleton,
} from "@/features/seller/SellerSkeletons";

/** /seller/analytics skeleton: header, range tabs, KPI row, funnel and revenue table. */
export default function SellerAnalyticsLoading() {
  return (
    <div className="space-y-6" aria-busy="true" data-testid="seller-skeleton">
      <HeaderSkeleton />
      <KpiRowSkeleton />
      <FormCardSkeleton lines={4} />
      <TableCardSkeleton rows={7} filter={false} />
    </div>
  );
}
