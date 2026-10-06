import {
  HeaderSkeleton,
  TableCardSkeleton,
} from "@/features/seller/SellerSkeletons";

/** /seller/orders skeleton: header, tabs + filter bar and table rows. */
export default function SellerOrdersLoading() {
  return (
    <div className="space-y-6" aria-busy="true" data-testid="seller-skeleton">
      <HeaderSkeleton />
      <TableCardSkeleton rows={8} />
    </div>
  );
}
