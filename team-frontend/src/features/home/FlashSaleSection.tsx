import Link from "next/link";

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
      className="scroll-mt-24 space-y-4 rounded-xl border border-border-subtle bg-surface-card p-4 shadow-preline-card"
    >
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border-subtle pb-3">
        <div className="flex items-center gap-3">
          <h2 className="text-xl font-semibold text-action-primary">
            Flash Sale
          </h2>
          {endsAt !== undefined && <CountdownClock endsAt={endsAt} />}
        </div>
        <Link
          href="/search"
          className="text-sm font-medium text-action-primary hover:underline"
        >
          Xem tất cả
        </Link>
      </div>

      <ul className="flex snap-x snap-mandatory gap-2 overflow-x-auto pb-2 sm:grid sm:grid-cols-3 sm:gap-3 sm:overflow-visible sm:pb-0 md:grid-cols-4 lg:grid-cols-6">
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
