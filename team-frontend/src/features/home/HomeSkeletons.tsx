import { range } from "@/components/ui/range";
import { ListingGridSkeleton } from "@/features/listing/ListingSkeleton";

const block = "animate-pulse bg-neutral-200";

/** Same footprint as Hero. */
export function HeroSkeleton() {
  return (
    <div
      aria-busy="true"
      className={`min-h-60 rounded-2xl sm:min-h-64 ${block}`}
    />
  );
}

/** Same footprint as ServiceHubs: 8 tiles, 4 per row on mobile. */
export function ServiceHubsSkeleton() {
  return (
    <div
      aria-busy="true"
      className="rounded-2xl border border-border-subtle bg-surface-card p-4 shadow-preline-card"
    >
      <div className="grid grid-cols-4 gap-2 sm:grid-cols-8">
        {range(8).map((i) => (
          <div
            key={i}
            className="flex min-h-20 flex-col items-center gap-1.5 p-2"
          >
            <div className={`h-12 w-12 rounded-2xl ${block}`} />
            <div className={`h-4 w-full rounded-xs ${block}`} />
          </div>
        ))}
      </div>
    </div>
  );
}

/** Same footprint as CategoryGridBlock. */
export function CategoryGridSkeleton() {
  return (
    <div
      aria-busy="true"
      className="overflow-hidden rounded-xl border border-border-subtle bg-surface-card shadow-preline-card"
    >
      <div className="border-b border-border-subtle px-5 py-4">
        <div className={`h-7 w-48 rounded-xs ${block}`} />
      </div>
      <div className="grid grid-cols-2 divide-x divide-y divide-border-subtle sm:grid-cols-5 md:grid-cols-10">
        {range(10).map((i) => (
          <div
            key={i}
            className="flex min-h-24 flex-col items-center justify-center gap-2 p-3"
          >
            <div className={`h-12 w-12 rounded-full ${block}`} />
            <div className={`h-4 w-3/4 rounded-xs ${block}`} />
          </div>
        ))}
      </div>
    </div>
  );
}

/** Heading bar + the real grid of card skeletons. */
export function FeedSkeleton({ count = 12 }: { count?: number }) {
  return (
    <div className="space-y-4" aria-busy="true">
      <div className={`h-16 rounded-2xl ${block}`} />
      <ListingGridSkeleton count={count} />
    </div>
  );
}

/** Below-the-fold row (recently viewed, recommendations): one row of cards. */
export function RowSkeleton() {
  return <FeedSkeleton count={6} />;
}
