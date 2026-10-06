import Link from "next/link";
import React from "react";
import { focusRing } from "./focus";

export interface PaginationProps {
  /** Current page, 1-based. */
  current: number;
  total: number;
  pageSize?: number;
  /** URL of a page; the component is plain links so it works in server components. */
  hrefFor: (page: number) => string;
  className?: string;
}

// Tier 3: component tokens.
const cell =
  "inline-flex h-9 min-w-9 items-center justify-center rounded-lg border px-2 text-sm transition duration-150";
const idle = `${cell} border-border-subtle bg-surface-card text-text-primary hover:bg-surface-page active:scale-95 ${focusRing}`;
const active = `${cell} border-action-primary bg-action-primary text-text-inverse`;
const disabled = `${cell} border-border-subtle bg-surface-card text-text-disabled opacity-50 pointer-events-none cursor-not-allowed`;

type Slot = number | "gap-start" | "gap-end";

/** Page numbers to show: all when few, otherwise first / current window / last. */
function slots(current: number, pages: number): Slot[] {
  if (pages <= 7) return Array.from({ length: pages }, (_, i) => i + 1);
  const out: Slot[] = [1];
  const from = Math.max(2, current - 1);
  const to = Math.min(pages - 1, current + 1);
  if (from > 2) out.push("gap-start");
  for (let p = from; p <= to; p++) out.push(p);
  if (to < pages - 1) out.push("gap-end");
  out.push(pages);
  return out;
}

/**
 * Ant Design `Pagination`, URL-driven: every page is a link built by
 * `hrefFor(page)`, the current one carries `aria-current="page"`. Renders
 * nothing when there is a single page.
 */
export function Pagination({
  current,
  total,
  pageSize = 10,
  hrefFor,
  className = "",
}: PaginationProps) {
  const pages = Math.max(1, Math.ceil(total / Math.max(1, pageSize)));
  if (pages <= 1) return null;
  const page = Math.min(Math.max(1, current), pages);

  return (
    <nav aria-label="Phân trang" className={className}>
      <ul className="flex flex-wrap items-center gap-1.5">
        <li>
          {page > 1 ? (
            <Link
              href={hrefFor(page - 1)}
              rel="prev"
              aria-label="Trang trước"
              className={idle}
            >
              ‹
            </Link>
          ) : (
            <span
              aria-disabled="true"
              aria-label="Trang trước"
              className={disabled}
            >
              ‹
            </span>
          )}
        </li>
        {slots(page, pages).map((slot) => (
          <li key={slot}>
            {typeof slot !== "number" ? (
              <span
                aria-hidden="true"
                className={`${cell} border-transparent text-text-disabled`}
              >
                …
              </span>
            ) : (
              <Link
                href={hrefFor(slot)}
                aria-label={`Trang ${slot}`}
                aria-current={slot === page ? "page" : undefined}
                className={slot === page ? `${active} ${focusRing}` : idle}
              >
                {slot}
              </Link>
            )}
          </li>
        ))}
        <li>
          {page < pages ? (
            <Link
              href={hrefFor(page + 1)}
              rel="next"
              aria-label="Trang sau"
              className={idle}
            >
              ›
            </Link>
          ) : (
            <span
              aria-disabled="true"
              aria-label="Trang sau"
              className={disabled}
            >
              ›
            </span>
          )}
        </li>
      </ul>
    </nav>
  );
}
