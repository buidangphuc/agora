import {
  FormCardSkeleton,
  HeaderSkeleton,
} from "@/features/seller/SellerSkeletons";

/** /seller/[id]/edit skeleton: header + the form's Card sections. */
export default function SellerEditLoading() {
  return (
    <div className="space-y-4" aria-busy="true" data-testid="seller-skeleton">
      <HeaderSkeleton />
      <FormCardSkeleton lines={3} />
      <FormCardSkeleton lines={6} />
      <FormCardSkeleton lines={4} />
      <FormCardSkeleton lines={3} />
    </div>
  );
}
