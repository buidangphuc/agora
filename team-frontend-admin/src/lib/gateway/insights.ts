import "server-only";

import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";

import type { GetExperimentReadoutResponse } from "@/generated/platform/analytics/v1/analytics_pb.js";

import { makeClients } from "./client.js";
import { getToken } from "./session.js";

type Params = Record<string, string | string[] | undefined>;

export type FlagParam =
  { kind: "none" } | { kind: "invalid" } | { kind: "valid"; key: string };

export interface InsightParams {
  windowHours: 24 | 72 | 168;
  weeks: 4 | 12 | 26;
  rangeDays: 7 | 14 | 30;
  flag: FlagParam;
}

const FLAG_PATTERN = /^[a-z0-9][a-z0-9_.-]{0,63}$/;
const TIMEOUT_MS = 5000;
const DAY_MS = 86_400_000;

function first(v: string | string[] | undefined): string | undefined {
  return Array.isArray(v) ? v[0] : v;
}

function pick<T extends number>(
  v: string | string[] | undefined,
  allowed: readonly T[],
  fallback: T,
): T {
  const raw = first(v);
  if (raw === undefined) return fallback;
  return allowed.find((a) => String(a) === raw) ?? fallback;
}

export function parseInsightParams(sp: Params): InsightParams {
  const rawFlag = first(sp.flag);
  let flag: FlagParam;
  if (rawFlag === undefined || rawFlag === "") flag = { kind: "none" };
  else if (FLAG_PATTERN.test(rawFlag)) flag = { kind: "valid", key: rawFlag };
  else flag = { kind: "invalid" };
  return {
    windowHours: pick(sp.window, [24, 72, 168] as const, 24),
    weeks: pick(sp.weeks, [4, 12, 26] as const, 12),
    rangeDays: pick(sp.range, [7, 14, 30] as const, 7),
    flag,
  };
}

export type ReportResult<T> =
  | { status: "ok"; data: T }
  | { status: "empty" }
  | { status: "unauthenticated" | "forbidden" | "invalid" | "unavailable" };

export type ExperimentResult<T> =
  ReportResult<T> | { status: "no_flag" } | { status: "bad_flag" };

type Clients = ReturnType<typeof makeClients>["analytics"];

async function run<T>(
  call: (a: Clients) => Promise<T>,
  rows: (r: T) => number,
): Promise<ReportResult<T>> {
  try {
    const { analytics } = makeClients(getToken());
    const res = await call(analytics);
    return rows(res) === 0 ? { status: "empty" } : { status: "ok", data: res };
  } catch (err) {
    if (err instanceof ConnectError) {
      if (err.code === Code.Unauthenticated)
        return { status: "unauthenticated" };
      if (err.code === Code.PermissionDenied) return { status: "forbidden" };
      if (err.code === Code.InvalidArgument) return { status: "invalid" };
    }
    return { status: "unavailable" };
  }
}

const opts = { timeoutMs: TIMEOUT_MS };

function range(days: number) {
  const now = Date.now();
  return {
    from: Timestamp.fromDate(new Date(now - days * DAY_MS)),
    to: Timestamp.fromDate(new Date(now)),
  };
}

export function fetchRecommendationPerformance(windowHours: number) {
  return run(
    (a) => a.getRecommendationPerformance({ windowHours }, opts),
    (r) => r.rows.length,
  );
}

export function fetchSearchPerformance(windowHours: number) {
  return run(
    (a) => a.getSearchPerformance({ windowHours }, opts),
    (r) => r.rows.length,
  );
}

export function fetchRetentionCohorts(weeks: number) {
  return run(
    (a) =>
      a.getRetentionCohorts(
        {
          weeks,
          from: Timestamp.fromDate(new Date(Date.now() - weeks * 7 * DAY_MS)),
        },
        opts,
      ),
    (r) => r.cohorts.length,
  );
}

export function fetchExperimentReadout(flag: string, rangeDays: number) {
  return run(
    (a) => a.getExperimentReadout({ flag, ...range(rangeDays) }, opts),
    (r) => r.variants.length,
  );
}

export function fetchAiSurfaceReport(rangeDays: number) {
  return run(
    (a) => a.getAiSurfaceReport({ ...range(rangeDays) }, opts),
    (r) => r.rows.length,
  );
}

export async function fetchInsights(params: InsightParams) {
  const experiment: Promise<ExperimentResult<GetExperimentReadoutResponse>> =
    params.flag.kind === "valid"
      ? fetchExperimentReadout(params.flag.key, params.rangeDays)
      : Promise.resolve(
          params.flag.kind === "none"
            ? { status: "no_flag" as const }
            : { status: "bad_flag" as const },
        );
  const [recommendation, search, retention, exp, aiSurfaces] =
    await Promise.all([
      fetchRecommendationPerformance(params.windowHours),
      fetchSearchPerformance(params.windowHours),
      fetchRetentionCohorts(params.weeks),
      experiment,
      fetchAiSurfaceReport(params.rangeDays),
    ]);
  return { recommendation, search, retention, experiment: exp, aiSurfaces };
}
