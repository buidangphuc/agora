import { Skeleton } from "@/components/ui/Skeleton";

/** Segment fallback: header + one content block (pages with their own footprint override it). */
export default function SellerLoading() {
  return (
    <div className="space-y-6" aria-busy="true" data-testid="seller-skeleton">
      <div className="space-y-3">
        <Skeleton variant="text" lines={1} className="w-48" />
        <Skeleton variant="text" lines={2} className="w-80" />
      </div>
      <Skeleton variant="text" lines={8} />
    </div>
  );
}
