import {
  HeaderSkeleton,
  KpiRowSkeleton,
  TableCardSkeleton,
} from "@/features/seller/SellerSkeletons";

/** /seller (Workplace) skeleton: header, KPI row, recent orders beside quick actions, product table. */
export default function SellerLoading() {
  return (
    <div className="space-y-8" aria-busy="true" data-testid="seller-skeleton">
      <HeaderSkeleton />
      <KpiRowSkeleton />
      <div className="grid gap-6 lg:grid-cols-3 lg:items-start lg:gap-8">
        <div className="min-w-0 lg:col-span-2">
          <TableCardSkeleton rows={5} filter={false} />
        </div>
        <TableCardSkeleton rows={5} filter={false} />
      </div>
      <TableCardSkeleton rows={8} />
    </div>
  );
}
