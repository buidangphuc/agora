import {
  FormCardSkeleton,
  HeaderSkeleton,
} from "@/features/seller/SellerSkeletons";

/** /seller/new skeleton: header + the form's Card sections. */
export default function SellerNewLoading() {
  return (
    <div className="space-y-6" aria-busy="true" data-testid="seller-skeleton">
      <HeaderSkeleton />
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3 lg:items-start">
        <div className="space-y-6 lg:col-span-2">
          <FormCardSkeleton lines={6} />
          <FormCardSkeleton lines={4} />
          <FormCardSkeleton lines={3} />
        </div>
        <FormCardSkeleton lines={3} />
      </div>
    </div>
  );
}
