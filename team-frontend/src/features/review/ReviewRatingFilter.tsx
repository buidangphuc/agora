"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { starCount } from "@/features/listing/pdp";
import type { ViewRatingBreakdown } from "@/lib/gateway/reviews";

const STARS = [5, 4, 3, 2, 1];

const base =
  "rounded-lg border px-3 py-1.5 text-sm font-medium transition duration-150 cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2";
const on = "border-text-primary bg-text-primary text-text-inverse";
const off =
  "border-border-strong bg-surface-card text-text-primary hover:bg-surface-muted";

/**
 * Star filter for the reviews list: All and 5..1 stars. Selecting writes
 * `?rating=` (and drops `?rpage=`) with `router.replace(..., { scroll: false })`,
 * keeping the other params and the `#reviews` hash, so the filter is shareable
 * and reloadable without history spam or a scroll jump.
 */
export function ReviewRatingFilter({
  total,
  breakdown,
  current,
}: {
  total: number;
  breakdown: ViewRatingBreakdown;
  /** Active filter: 0 = all, otherwise 1..5. */
  current: number;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  function apply(star: number) {
    if (star === current) return;
    const params = new URLSearchParams(searchParams?.toString() ?? "");
    if (star === 0) params.delete("rating");
    else params.set("rating", String(star));
    params.delete("rpage");
    const qs = params.toString();
    router.replace(`${pathname}${qs ? `?${qs}` : ""}#reviews`, {
      scroll: false,
    });
  }

  return (
    <div className="flex flex-wrap gap-2">
      <button
        type="button"
        data-testid="review-filter"
        aria-pressed={current === 0}
        onClick={() => apply(0)}
        className={`${base} ${current === 0 ? on : off}`}
      >
        Tất cả ({total})
      </button>
      {STARS.map((star) => (
        <button
          key={star}
          type="button"
          data-testid="review-filter"
          aria-pressed={current === star}
          onClick={() => apply(star)}
          className={`${base} ${current === star ? on : off}`}
        >
          {star} Sao ({starCount(breakdown, star)})
        </button>
      ))}
    </div>
  );
}
