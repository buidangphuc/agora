import { Card } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";
import { ListingGridSkeleton } from "@/features/listing/ListingSkeleton";

const block = "animate-pulse bg-neutral-200";

/** Filter column placeholder: a button on mobile, the filter card from 1024px. */
export function FilterSkeleton() {
  return (
    <div aria-busy="true">
      <div className={`h-11 w-full rounded-lg lg:hidden ${block}`} />
      <Card className="hidden p-4 lg:block">
        <Skeleton variant="text" lines={8} />
      </Card>
    </div>
  );
}

/** Results column placeholder: the sort bar footprint and 24 card skeletons. */
export function ResultsSkeleton() {
  return (
    <div className="space-y-4" aria-busy="true">
      <div className={`h-16 rounded-xl ${block}`} />
      <ListingGridSkeleton count={24} />
    </div>
  );
}

/** Title block placeholder (breadcrumb, h1, count). */
export function SearchHeaderSkeleton() {
  return (
    <div className="space-y-2" aria-busy="true">
      <div className={`h-4 w-48 rounded-xs ${block}`} />
      <div className={`h-7 w-64 rounded-xs ${block}`} />
      <div className={`h-5 w-32 rounded-xs ${block}`} />
    </div>
  );
}
