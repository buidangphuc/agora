import {
  HeaderSkeleton,
  TableCardSkeleton,
} from "@/features/seller/SellerSkeletons";

export default function MarketingLoading() {
  return (
    <div className="space-y-6" aria-busy="true" data-testid="seller-skeleton">
      <HeaderSkeleton />
      <TableCardSkeleton rows={4} filter={false} />
    </div>
  );
}
