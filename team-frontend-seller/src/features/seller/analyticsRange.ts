import type { ViewDayRevenue } from "@/lib/gateway/analytics";

export const RANGES = ["7d", "30d", "90d"] as const;
export type Range = (typeof RANGES)[number];
export const DEFAULT_RANGE: Range = "7d";

export const RANGE_LABEL: Record<Range, string> = {
  "7d": "7 ngày",
  "30d": "30 ngày",
  "90d": "90 ngày",
};

const DAYS: Record<Range, number> = { "7d": 7, "30d": 30, "90d": 90 };
const DAY_MS = 24 * 60 * 60 * 1000;

/** `?range=` with a safe default for anything unknown. */
export function parseRange(value: string | string[] | undefined): Range {
  const v = Array.isArray(value) ? value[0] : value;
  return (RANGES as readonly string[]).includes(v ?? "")
    ? (v as Range)
    : DEFAULT_RANGE;
}

/** Revenue days inside the last N days (a day that cannot be parsed is kept). */
export function daysInRange(
  days: ViewDayRevenue[],
  range: Range,
  now: number = Date.now(),
): ViewDayRevenue[] {
  const cutoff = now - DAYS[range] * DAY_MS;
  return days.filter((d) => {
    const t = Date.parse(d.day);
    return Number.isNaN(t) || t >= cutoff;
  });
}

/** Funnel step as a percentage of impressions (impressions = 100). */
export function ratio(step: number, impressions: number): number {
  return impressions > 0 ? Math.min(100, (step / impressions) * 100) : 0;
}
