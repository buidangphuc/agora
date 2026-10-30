// Pure mapping from the /admin/audit query string to gateway request filters.
// Day bounds are calendar days in Vietnam time (UTC+07:00); `to` is inclusive,
// so the exclusive upper bound is the start of the following day.

export type SearchParams = Record<string, string | string[] | undefined>;

export interface AuditFilters {
  targetType?: string;
  targetId?: string;
  actorId?: string;
  action?: string;
  createdAfter?: Date;
  createdBefore?: Date;
}

export interface AuditParams {
  target_type?: string;
  target_id?: string;
  actor_id?: string;
  action?: string;
  from?: string;
  to?: string;
}

export interface ParsedAuditFilters {
  filters: AuditFilters;
  cursor: string;
  params: AuditParams;
  error?: "invalid_date";
}

const MAX_VALUE = 128;
const MAX_CURSOR = 256;
const VN_OFFSET_MS = 7 * 60 * 60 * 1000;
const DATE_RE = /^(\d{4})-(\d{2})-(\d{2})$/;

function clean(
  raw: string | string[] | undefined,
  max: number,
): string | undefined {
  const v = (Array.isArray(raw) ? raw[0] : raw)?.trim().slice(0, max);
  return v ? v : undefined;
}

// Start of the calendar day `dayShift` days after `ymd`, in UTC+07:00.
function vnDayStart(ymd: string, dayShift: number): Date | null {
  const m = DATE_RE.exec(ymd);
  if (!m) return null;
  const [y, mo, d] = [Number(m[1]), Number(m[2]), Number(m[3])];
  const check = new Date(Date.UTC(y, mo - 1, d));
  if (
    check.getUTCFullYear() !== y ||
    check.getUTCMonth() !== mo - 1 ||
    check.getUTCDate() !== d
  ) {
    return null;
  }
  return new Date(Date.UTC(y, mo - 1, d + dayShift) - VN_OFFSET_MS);
}

export function parseAuditFilters(
  searchParams: SearchParams,
): ParsedAuditFilters {
  const params: AuditParams = {};
  const filters: AuditFilters = {};
  let error: "invalid_date" | undefined;

  const targetType = clean(searchParams.target_type, MAX_VALUE);
  const targetId = clean(searchParams.target_id, MAX_VALUE);
  const actorId = clean(searchParams.actor_id, MAX_VALUE);
  const action = clean(searchParams.action, MAX_VALUE);
  if (targetType) {
    params.target_type = targetType;
    filters.targetType = targetType;
  }
  if (targetId) {
    params.target_id = targetId;
    filters.targetId = targetId;
  }
  if (actorId) {
    params.actor_id = actorId;
    filters.actorId = actorId;
  }
  if (action) {
    params.action = action;
    filters.action = action;
  }

  const from = clean(searchParams.from, MAX_VALUE);
  if (from) {
    params.from = from;
    const d = vnDayStart(from, 0);
    if (d) filters.createdAfter = d;
    else error = "invalid_date";
  }
  const to = clean(searchParams.to, MAX_VALUE);
  if (to) {
    params.to = to;
    const d = vnDayStart(to, 1);
    if (d) filters.createdBefore = d;
    else error = "invalid_date";
  }

  const cursor = clean(searchParams.cursor, MAX_CURSOR) ?? "";
  return error
    ? { filters, cursor, params, error }
    : { filters, cursor, params };
}
