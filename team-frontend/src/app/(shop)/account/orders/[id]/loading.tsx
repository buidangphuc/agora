import { Card } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";

/** Same footprint as the detail: header, progress band, descriptions, table. */
export default function OrderDetailLoading() {
  return (
    <section
      className="mx-auto flex w-full max-w-5xl flex-col gap-6 py-4 md:gap-8 md:py-8"
      aria-busy="true"
      aria-live="polite"
    >
      <Card padded>
        <div className="flex flex-col gap-4">
          <div className="h-4 w-40 max-w-full animate-pulse rounded-xl bg-neutral-200" />
          <div className="h-9 w-64 max-w-full animate-pulse rounded-xl bg-neutral-200" />
          <div className="h-9 w-32 animate-pulse rounded-xl bg-neutral-200" />
        </div>
      </Card>
      <Card padded>
        <Skeleton variant="text" lines={2} />
      </Card>
      <div className="grid grid-cols-1 gap-6 md:gap-8 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <Card padded>
            <Skeleton variant="text" lines={5} />
          </Card>
        </div>
        <Card padded>
          <Skeleton variant="text" lines={4} />
        </Card>
      </div>
    </section>
  );
}
