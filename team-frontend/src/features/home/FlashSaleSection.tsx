import { Link } from "@/components/ui/Link";
import { Progress } from "@/components/ui/Progress";
import { ListingCard } from "@/features/listing/ListingCard";
import type { ViewListing } from "@/lib/gateway/listings";
import { CountdownClock } from "./CountdownClock";

/** Real per-item campaign figures; items without an entry get no progress bar. */
export interface FlashSaleStock {
  sold: number;
  stock: number;
}

/**
 * Flash-sale block, rendered only from REAL campaign data: the listings, a
 * real `endsAt` (the countdown mounts only with it) and real sold/stock per
 * item (the Progress bar shows only for those). Renders nothing for an empty
 * list. A server component: only CountdownClock ships client code.
 */
export function FlashSaleSection({
  listings,
  endsAt,
  stockById,
}: {
  listings: ViewListing[];
  endsAt?: string | number;
  stockById?: Record<string, FlashSaleStock>;
}) {
  if (listings.length === 0) return null;

  return (
    <section
      id="flash-sale"
      className="scroll-mt-24 space-y-5 rounded-3xl border border-border-subtle bg-surface-card p-4 shadow-card sm:p-6"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h2 className="text-xl font-semibold tracking-tight text-text-primary sm:text-2xl">
            Flash Sale
          </h2>
          {endsAt !== undefined && <CountdownClock endsAt={endsAt} />}
        </div>
        <Link href="/search" variant="ghost" size="sm">
          Xem tất cả
        </Link>
      </div>

      <ul className="-mx-4 flex snap-x snap-mandatory gap-3 overflow-x-auto px-4 pb-2 sm:mx-0 sm:grid sm:grid-cols-3 sm:gap-4 sm:overflow-visible sm:px-0 sm:pb-0 md:grid-cols-4 lg:grid-cols-6">
        {listings.slice(0, 6).map((l, i) => {
          const figures = stockById?.[l.id];
          return (
            <li
              key={l.id}
              className="w-40 shrink-0 snap-start space-y-2 sm:w-auto"
            >
              <ListingCard listing={l} position={i + 1} />
              {figures !== undefined && figures.stock > 0 && (
                <Progress
                  size="sm"
                  label="Đã bán"
                  percent={(figures.sold / figures.stock) * 100}
                />
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
