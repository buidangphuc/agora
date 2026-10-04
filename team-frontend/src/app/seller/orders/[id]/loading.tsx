import {
  FormCardSkeleton,
  HeaderSkeleton,
} from "@/features/seller/SellerSkeletons";

/** /seller/orders/[id] skeleton: header, summary, stepper, items and recipient cards. */
export default function SellerOrderDetailLoading() {
  return (
    <div className="space-y-6" aria-busy="true" data-testid="seller-skeleton">
      <HeaderSkeleton />
      <FormCardSkeleton lines={4} />
      <FormCardSkeleton lines={2} />
      <FormCardSkeleton lines={6} />
      <FormCardSkeleton lines={3} />
    </div>
  );
}
