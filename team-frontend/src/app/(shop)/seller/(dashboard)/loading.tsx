import {
  HeaderSkeleton,
  KpiRowSkeleton,
  TableCardSkeleton,
} from "@/features/seller/SellerSkeletons";

/** /seller (Workplace) skeleton: header, KPI row, quick actions, two tables. */
export default function SellerLoading() {
  return (
    <div className="space-y-6" aria-busy="true" data-testid="seller-skeleton">
      <HeaderSkeleton />
      <KpiRowSkeleton />
      <TableCardSkeleton rows={2} filter={false} />
      <TableCardSkeleton rows={5} filter={false} />
      <TableCardSkeleton rows={8} />
    </div>
  );
}
