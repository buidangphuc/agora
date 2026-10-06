import {
  FormCardSkeleton,
  HeaderSkeleton,
} from "@/features/seller/SellerSkeletons";

const PLACEHOLDERS = ["p1", "p2", "p3"];

/** /seller/plans skeleton: header and a three-card grid. */
export default function SellerPlansLoading() {
  return (
    <div className="space-y-6" aria-busy="true" data-testid="seller-skeleton">
      <HeaderSkeleton />
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {PLACEHOLDERS.map((key) => (
          <FormCardSkeleton key={key} lines={6} />
        ))}
      </div>
    </div>
  );
}
