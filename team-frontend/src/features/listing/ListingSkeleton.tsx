import { Skeleton } from "@/components/ui/Skeleton";
import { range } from "@/components/ui/range";
import { LISTING_GRID_CLASS } from "./gridClass";

/**
 * Placeholder with the same footprint as ListingCard: same border, padding,
 * 1:1 image box, two-line title, one meta row and one price row.
 */
export function ListingCardSkeleton() {
  return (
    <div
      aria-busy="true"
      className="flex h-full flex-col overflow-hidden rounded-xl border border-border-subtle bg-surface-card shadow-preline-card"
    >
      <Skeleton variant="image" aspect="square" />
      <div className="flex flex-1 flex-col gap-2 p-3">
        <Skeleton variant="text" lines={2} />
        <div className="min-h-5" />
        <div className="h-5 w-1/2 animate-pulse rounded-xs bg-neutral-200" />
      </div>
    </div>
  );
}

/** The product grid of ListingCardSkeleton, in the exact grid of ListingGrid. */
export function ListingGridSkeleton({ count = 12 }: { count?: number }) {
  return (
    <div className={LISTING_GRID_CLASS} aria-busy="true">
      {range(count).map((i) => (
        <ListingCardSkeleton key={i} />
      ))}
    </div>
  );
}
