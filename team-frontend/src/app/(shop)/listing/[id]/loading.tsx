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
      className="space-y-4"
      aria-busy="true"
      aria-live="polite"
      data-testid="pdp-skeleton"
    >
      {/* Breadcrumb footprint: one text-xs line at lg, two wrapped lines below. */}
      <div className="space-y-1.5" data-testid="pdp-skeleton-breadcrumb">
        <div className="h-4 w-2/3 animate-pulse rounded-xs bg-neutral-200 lg:w-1/3" />
        <div className="h-4 w-1/2 animate-pulse rounded-xs bg-neutral-200 lg:hidden" />
      </div>

      <Card className="p-6">
        <div className="grid grid-cols-1 gap-8 lg:grid-cols-12">
          <div className="space-y-3 lg:col-span-5">
            <Skeleton variant="image" aspect="square" />
            <div className="flex gap-2">
              <Skeleton variant="avatar" size="lg" />
              <Skeleton variant="avatar" size="lg" />
              <Skeleton variant="avatar" size="lg" />
            </div>
          </div>
          <div className="space-y-4 lg:col-span-7">
            <Skeleton variant="text" lines={2} />
            <Skeleton variant="text" lines={1} className="w-1/3" />
            <Skeleton variant="image" aspect="4/3" className="max-h-24" />
            <Skeleton variant="text" lines={3} />
            <div className="flex gap-3">
              <Skeleton variant="text" lines={1} className="w-40" />
              <Skeleton variant="text" lines={1} className="w-40" />
            </div>
          </div>
        </div>
      </Card>

      <Card className="p-5">
        <div className="flex items-center gap-4">
          <Skeleton variant="avatar" size="lg" />
          <Skeleton variant="text" lines={2} className="flex-1" />
        </div>
      </Card>

      <Card className="p-5">
        <Skeleton variant="text" lines={1} className="w-1/2" />
      </Card>

      <Card className="p-6">
        <Skeleton variant="text" lines={5} />
      </Card>
    </div>
  );
}
