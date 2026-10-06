import Link from "next/link";

import { focusRing } from "@/components/ui/focus";

const cell =
  "inline-flex h-11 min-w-11 items-center justify-center rounded-lg border px-3 text-sm transition duration-150";
const idle = `${cell} border-border-subtle bg-surface-card text-text-primary hover:bg-surface-page ${focusRing}`;
const off = `${cell} pointer-events-none cursor-not-allowed border-border-subtle bg-surface-card text-text-disabled opacity-50`;

/**
 * Pagination for 375px: previous, the current page and next only. Plain links
 * (server-compatible); the numbered Pagination takes over from `sm` up.
 */
export function CompactPagination({
  current,
  total,
  pageSize,
  hrefFor,
  className = "",
}: {
  current: number;
  total: number;
  pageSize: number;
  hrefFor: (page: number) => string;
  className?: string;
}) {
  const pages = Math.max(1, Math.ceil(total / Math.max(1, pageSize)));
  if (pages <= 1) return null;
  const page = Math.min(Math.max(1, current), pages);

  return (
    <nav aria-label="Phân trang" className={className}>
      <ul className="flex items-center justify-center gap-2">
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
            <span aria-disabled="true" aria-label="Trang trước" className={off}>
              ‹
            </span>
          )}
        </li>
        <li>
          <span
            aria-current="page"
            className={`${cell} border-action-primary bg-action-primary text-text-inverse`}
          >
            {page}
          </span>
        </li>
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
            <span aria-disabled="true" aria-label="Trang sau" className={off}>
              ›
            </span>
          )}
        </li>
      </ul>
    </nav>
  );
}
