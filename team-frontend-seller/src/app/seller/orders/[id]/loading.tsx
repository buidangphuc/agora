import {
  FormCardSkeleton,
  HeaderSkeleton,
} from "@/features/seller/SellerSkeletons";

/** /seller/orders/[id] skeleton: header, summary column beside the stepper and items cards. */
export default function SellerOrderDetailLoading() {
  return (
    <div className="space-y-8" aria-busy="true" data-testid="seller-skeleton">
      <HeaderSkeleton />
      <div className="grid gap-6 lg:grid-cols-3 lg:items-start lg:gap-8">
        <div className="min-w-0 space-y-6 lg:col-start-3 lg:row-start-1">
          <FormCardSkeleton lines={2} />
          <FormCardSkeleton lines={5} />
          <FormCardSkeleton lines={3} />
        </div>
        <div className="min-w-0 space-y-6 lg:col-span-2 lg:col-start-1 lg:row-start-1">
          <FormCardSkeleton lines={2} />
          <FormCardSkeleton lines={6} />
        </div>
      </div>
    </div>
  );
}
