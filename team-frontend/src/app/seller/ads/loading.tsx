import {
  FormCardSkeleton,
  HeaderSkeleton,
  TableCardSkeleton,
} from "@/features/seller/SellerSkeletons";

/** /seller/ads skeleton: header, form card and list card. */
export default function SellerAdsLoading() {
  return (
    <div className="space-y-6" aria-busy="true" data-testid="seller-skeleton">
      <HeaderSkeleton />
      <FormCardSkeleton lines={6} />
      <TableCardSkeleton rows={3} filter={false} />
    </div>
  );
}
