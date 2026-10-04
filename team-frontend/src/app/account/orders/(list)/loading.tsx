import { Card } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";

const ROWS = ["a", "b", "c"];

/** Same footprint as the list: title, tab bar and three order cards. */
export default function OrdersLoading() {
  return (
    <section
      className="mx-auto max-w-4xl space-y-4 py-2"
      aria-busy="true"
      aria-live="polite"
    >
      <header className="space-y-2">
        <div className="h-7 w-48 animate-pulse rounded-lg bg-neutral-200" />
        <div className="h-5 w-24 animate-pulse rounded-lg bg-neutral-200" />
      </header>
      <div className="h-11 w-full animate-pulse rounded-lg bg-neutral-200" />
      {ROWS.map((id) => (
        <Card key={id}>
          <div className="space-y-4 p-4">
            <Skeleton variant="text" lines={1} />
            <div className="flex gap-3">
              <div className="w-16 shrink-0">
                <Skeleton variant="image" aspect="square" />
              </div>
              <div className="flex-1">
                <Skeleton variant="text" lines={3} />
              </div>
            </div>
            <div className="h-9 w-full animate-pulse rounded-lg bg-neutral-200" />
          </div>
        </Card>
      ))}
    </section>
  );
}
