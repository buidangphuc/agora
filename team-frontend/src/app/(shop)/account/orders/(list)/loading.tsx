import { Card } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";

const ROWS = ["a", "b", "c"];

/** Same footprint as the list: title, tab bar and three order cards. */
export default function OrdersLoading() {
  return (
    <section
      className="mx-auto flex w-full max-w-4xl flex-col gap-6 py-4 md:gap-8 md:py-8"
      aria-busy="true"
      aria-live="polite"
    >
      <header className="flex flex-col gap-2">
        <div className="h-8 w-48 animate-pulse rounded-xl bg-neutral-200 md:h-9" />
        <div className="h-6 w-24 animate-pulse rounded-xl bg-neutral-200" />
      </header>
      <div className="h-11 w-full animate-pulse rounded-xl bg-neutral-200" />
      {ROWS.map((id) => (
        <Card key={id}>
          <div className="flex flex-col gap-4 p-4 sm:p-6">
            <Skeleton variant="text" lines={1} />
            <div className="flex gap-4">
              <div className="w-16 shrink-0 sm:w-20">
                <Skeleton variant="image" aspect="square" />
              </div>
              <div className="flex-1">
                <Skeleton variant="text" lines={3} />
              </div>
            </div>
            <div className="h-11 w-full animate-pulse rounded-xl bg-neutral-200" />
          </div>
        </Card>
      ))}
    </section>
  );
}
