import { Skeleton } from "@/components/ui/Skeleton";
import { range } from "@/components/ui/range";

/**
 * Footprint of one recommendations row (heading + one grid row, matching
 * ListingGrid's columns) while team-ai answers. Streamed away without a
 * leftover when the service returns nothing.
 */
export function RecommendationsSkeleton() {
  return (
    <section aria-busy="true" className="space-y-3">
      <Skeleton variant="text" lines={1} className="w-1/3" />
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
        {range(6).map((i) => (
          <Skeleton key={i} variant="card" aspect="square" />
        ))}
      </div>
    </section>
  );
}
