import { Card } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";
import { ListingGridSkeleton } from "@/features/listing/ListingSkeleton";

const block = "animate-pulse bg-surface-muted";

/** Filter column placeholder: a button on mobile, the filter card from 1024px. */
export function FilterSkeleton() {
  return (
    <div aria-busy="true">
      <div className={`h-11 w-full rounded-xl lg:hidden ${block}`} />
      <div className="hidden lg:block">
        <Card padded>
          <Skeleton variant="text" lines={8} />
        </Card>
      </div>
    </div>
  );
}

/** Results column placeholder: the sort bar footprint and 24 card skeletons. */
export function ResultsSkeleton() {
  return (
    <div className="space-y-4" aria-busy="true">
      <div className={`h-24 rounded-2xl sm:h-20 ${block}`} />
      <ListingGridSkeleton count={24} />
    </div>
  );
}

/** Title block placeholder (breadcrumb, h1, count). */
export function SearchHeaderSkeleton() {
  return (
    <div className="space-y-3" aria-busy="true">
      <div className={`h-4 w-48 max-w-full rounded-lg ${block}`} />
      <div className={`h-8 w-64 max-w-full rounded-lg md:h-9 ${block}`} />
      <div className={`h-5 w-32 rounded-lg ${block}`} />
    </div>
  );
}
