/**
 * URL state of the seller list pages (`?q=&status=&page=`). Every value falls
 * back to a safe default: an invalid URL never throws.
 */
export type SearchParams = Record<string, string | string[] | undefined>;

export interface ListParams {
  q: string;
  status: string;
  page: number;
}

export const DEFAULT_STATUS = "all";
export const MAX_QUERY_LENGTH = 100;
const MAX_PAGE = 1000;

function first(value: string | string[] | undefined): string {
  return (Array.isArray(value) ? value[0] : value) ?? "";
}

/**
 * Parse q, status and page. `statuses` lists the accepted status values
 * (besides "all"); anything else becomes "all". `page` must be an integer
 * >= 1, otherwise 1.
 */
export function parseListParams(
  searchParams: SearchParams = {},
  statuses: readonly string[] = [],
): ListParams {
  const q = first(searchParams.q).trim().slice(0, MAX_QUERY_LENGTH);
  const rawStatus = first(searchParams.status);
  const status = statuses.includes(rawStatus) ? rawStatus : DEFAULT_STATUS;
  const rawPage = first(searchParams.page);
  const parsed = /^\d+$/.test(rawPage) ? Number.parseInt(rawPage, 10) : 1;
  const page = Math.min(MAX_PAGE, Math.max(1, parsed));
  return { q, status, page };
}

/** `/path?q=..&status=..&page=..` omitting empty and default values. */
export function buildListHref(basePath: string, params: object): string {
  const qs = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === "") continue;
    if (key === "status" && value === DEFAULT_STATUS) continue;
    if (key === "page" && Number(value) <= 1) continue;
    qs.set(key, String(value));
  }
  const text = qs.toString();
  return text ? `${basePath}?${text}` : basePath;
}

/** Items of the 1-based `page` of a fully fetched list. */
export function slicePage<T>(items: T[], page: number, pageSize: number): T[] {
  const start = (Math.max(1, page) - 1) * pageSize;
  return items.slice(start, start + pageSize);
}
