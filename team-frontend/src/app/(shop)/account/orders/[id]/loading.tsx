import { Card } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";

/** Same footprint as the detail: header, progress band, descriptions, table. */
export default function OrderDetailLoading() {
  return (
    <section
      className="mx-auto max-w-4xl space-y-6 py-2"
      aria-busy="true"
      aria-live="polite"
    >
      <Card padded>
        <div className="space-y-3">
          <div className="h-4 w-40 animate-pulse rounded-lg bg-neutral-200" />
          <div className="h-8 w-64 animate-pulse rounded-lg bg-neutral-200" />
          <div className="h-8 w-32 animate-pulse rounded-lg bg-neutral-200" />
        </div>
      </Card>
      <Card padded>
        <Skeleton variant="text" lines={2} />
      </Card>
      <Card padded>
        <Skeleton variant="text" lines={4} />
      </Card>
      <Card padded>
        <Skeleton variant="text" lines={5} />
      </Card>
    </section>
  );
}
