const QUERY_MAX_BYTES = 64;
const CURSOR_MAX_CHARS = 256;
const STATUSES = ["active", "restricted", "suspended", "banned"] as const;

type Param = string | string[] | undefined;

export type UserStatus = (typeof STATUSES)[number];

export interface UserFilters {
  query: string;
  status: UserStatus | undefined;
  cursor: string;
}

function first(v: Param): string {
  const s = Array.isArray(v) ? v[0] : v;
  return (s ?? "").trim();
}

function capBytes(s: string, max: number): string {
  const enc = new TextEncoder();
  let out = "";
  let used = 0;
  for (const ch of s) {
    const n = enc.encode(ch).length;
    if (used + n > max) break;
    out += ch;
    used += n;
  }
  return out;
}

export function parseUserFilters(
  searchParams: Record<string, Param>,
): UserFilters {
  const rawStatus = first(searchParams.status).toLowerCase();
  const status = (STATUSES as readonly string[]).includes(rawStatus)
    ? (rawStatus as UserStatus)
    : undefined;
  return {
    query: capBytes(first(searchParams.query), QUERY_MAX_BYTES),
    status,
    cursor: first(searchParams.cursor).slice(0, CURSOR_MAX_CHARS),
  };
}
