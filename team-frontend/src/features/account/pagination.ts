/** Rows per page for URL-paginated account lists (`?page=N`). */
export const PAGE_SIZE = 20;

/** `?page=` value to a 1-based page number; anything invalid is page 1. */
export function parsePage(value: string | undefined): number {
  const page = Number.parseInt(value ?? "", 10);
  return Number.isFinite(page) && page > 0 ? page : 1;
}

/** The rows of `page`, clamped to the last page. */
export function paginate<T>(
  items: T[],
  page: number,
  pageSize = PAGE_SIZE,
): { rows: T[]; page: number } {
  const pages = Math.max(1, Math.ceil(items.length / pageSize));
  const current = Math.min(page, pages);
  return {
    rows: items.slice((current - 1) * pageSize, current * pageSize),
    page: current,
  };
}
