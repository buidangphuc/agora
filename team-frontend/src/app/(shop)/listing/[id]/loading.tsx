import { Card } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";

/**
 * Skeleton of the whole PDP anatomy with the footprints of the final layout
 * (breadcrumb, gallery 1:1, title / price / actions, shop card, anchor nav), so
 * content swaps in without layout shift.
 */
export default function Loading() {
  return (
    <div
      className="space-y-6 md:space-y-8"
      aria-busy="true"
      aria-live="polite"
      data-testid="pdp-skeleton"
    >
      <div className="space-y-1.5" data-testid="pdp-skeleton-breadcrumb">
        <div className="h-4 w-2/3 animate-pulse rounded-xs bg-neutral-200 lg:w-1/3" />
        <div className="h-4 w-1/2 animate-pulse rounded-xs bg-neutral-200 lg:hidden" />
      </div>

      <div className="grid grid-cols-1 gap-6 md:grid-cols-2 md:gap-8 lg:grid-cols-12 lg:gap-10">
        <div className="min-w-0 space-y-4 lg:col-span-6">
          <Skeleton variant="image" aspect="square" />
          <div className="flex gap-3">
            <Skeleton variant="avatar" size="lg" />
            <Skeleton variant="avatar" size="lg" />
            <Skeleton variant="avatar" size="lg" />
          </div>
        </div>
        <div className="min-w-0 lg:col-span-6">
          <Card padded>
            <div className="space-y-6">
              <Skeleton variant="text" lines={2} />
              <div className="w-1/3">
                <Skeleton variant="text" lines={1} />
              </div>
              <div className="max-h-24">
                <Skeleton variant="image" aspect="4/3" />
              </div>
              <Skeleton variant="text" lines={3} />
              <div className="flex flex-wrap gap-3">
                <div className="w-40">
                  <Skeleton variant="text" lines={1} />
                </div>
                <div className="w-40">
                  <Skeleton variant="text" lines={1} />
                </div>
              </div>
            </div>
          </Card>
        </div>
      </div>

      <Card padded>
        <div className="flex items-center gap-4">
          <Skeleton variant="avatar" size="lg" />
          <div className="flex-1">
            <Skeleton variant="text" lines={2} />
          </div>
        </div>
      </Card>

      <Card padded>
        <div className="w-1/2">
          <Skeleton variant="text" lines={1} />
        </div>
      </Card>

      <Card padded>
        <Skeleton variant="text" lines={5} />
      </Card>
    </div>
  );
}
