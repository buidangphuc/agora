import { Link } from "@/components/ui/Link";
import { Pagination } from "@/components/ui/Pagination";
import { focusRing } from "@/components/ui/focus";

const compactOff =
  "inline-flex h-9 items-center rounded-lg border border-border-subtle bg-surface-card px-3 text-sm text-text-disabled opacity-50 pointer-events-none";

/**
 * Pagination for the seller tables: full page links from 768px up, and only
 * previous / current / next on a phone.
 */
export function SellerPagination({
  current,
  total,
  pageSize,
  hrefFor,
}: {
  current: number;
  total: number;
  pageSize: number;
  hrefFor: (page: number) => string;
}) {
  const pages = Math.max(1, Math.ceil(total / Math.max(1, pageSize)));
  if (pages <= 1) return null;
  const page = Math.min(Math.max(1, current), pages);
  return (
    <>
      <div className="hidden md:block">
        <Pagination
          current={page}
          total={total}
          pageSize={pageSize}
          hrefFor={hrefFor}
        />
      </div>
      <nav
        aria-label="Phân trang (di động)"
        className="flex items-center justify-between gap-3 md:hidden"
      >
        {page > 1 ? (
          <Link href={hrefFor(page - 1)} rel="prev" variant="outline" size="xs">
            Trước
          </Link>
        ) : (
          <span aria-disabled="true" className={compactOff}>
            Trước
          </span>
        )}
        <span className="text-sm text-text-secondary" aria-current="page">
          Trang {page}/{pages}
        </span>
        {page < pages ? (
          <Link href={hrefFor(page + 1)} rel="next" variant="outline" size="xs">
            Sau
          </Link>
        ) : (
          <span aria-disabled="true" className={compactOff}>
            Sau
          </span>
        )}
      </nav>
    </>
  );
}
