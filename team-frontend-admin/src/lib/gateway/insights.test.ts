import { Code, ConnectError } from "@connectrpc/connect";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { makeClients } from "./client.js";
import {
  fetchAiSurfaceReport,
  fetchExperimentReadout,
  fetchInsights,
  fetchRecommendationPerformance,
  fetchRetentionCohorts,
  fetchSearchPerformance,
  parseInsightParams,
} from "./insights.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

// Contract under test (implementer creates src/lib/gateway/insights.ts):
//   parseInsightParams(sp: Record<string, string | string[] | undefined>) => {
//     windowHours: 24 | 72 | 168;   // default 24, unknown -> 24
//     weeks: 4 | 12 | 26;           // default 12, unknown -> 12
//     rangeDays: 7 | 14 | 30;       // default 7, unknown -> 7
//     flag: { kind: "none" } | { kind: "invalid" } | { kind: "valid"; key: string };
//   }   flag pattern ^[a-z0-9][a-z0-9_.-]{0,63}$ ; absent or empty -> none
//   fetchRecommendationPerformance(windowHours), fetchSearchPerformance(windowHours),
//   fetchRetentionCohorts(weeks), fetchExperimentReadout(key, rangeDays),
//   fetchAiSurfaceReport(rangeDays) =>
//     { status: "ok"; data: <raw response> } | { status: "empty" } |
//     { status: "unauthenticated" | "forbidden" | "invalid" | "unavailable" }
//     "empty" = ok response with zero rows (rows / cohorts / variants).
//   fetchInsights(params: ReturnType<typeof parseInsightParams>) => Promise<{
//     recommendation, search, retention, experiment, aiSurfaces }>
//     experiment is { status: "no_flag" } or { status: "bad_flag" } without a call.
//   Calls use makeClients(token).analytics, a 5 s timeoutMs, run in parallel
//   and results never carry err.message. Time comes from Date.now().

type Fn = ReturnType<typeof vi.fn>;
const DAY = 86_400_000;
const NOW = new Date("2026-11-14T10:00:00Z");

function stub(over: Partial<Record<string, Fn>> = {}) {
  const analytics = {
    getRecommendationPerformance: vi.fn().mockResolvedValue({
      rows: [{ placementId: "home_feed" }],
      fallback: [],
    }),
    getSearchPerformance: vi
      .fn()
      .mockResolvedValue({ rows: [{ placementId: "search" }] }),
    getRetentionCohorts: vi
      .fn()
      .mockResolvedValue({ cohorts: [{ cohortWeek: "2026-11-02" }] }),
    getExperimentReadout: vi
      .fn()
      .mockResolvedValue({ variants: [{ variant: "control" }], windowDays: 7 }),
    getAiSurfaceReport: vi
      .fn()
      .mockResolvedValue({ rows: [{ surface: "assistant" }] }),
    ...over,
  };
  vi.mocked(makeClients).mockReturnValue({ analytics } as never);
  return analytics;
}

const defaults = () => parseInsightParams({});

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
});
afterEach(() => vi.useRealTimers());

describe("parseInsightParams", () => {
  it("defaults to 24 hours, 12 weeks, 7 days and no flag", () => {
    expect(defaults()).toEqual({
      windowHours: 24,
      weeks: 12,
      rangeDays: 7,
      flag: { kind: "none" },
    });
  });

  // Scenario: Window selection is sent as hours
  it("accepts 72 and 168 for window", () => {
    expect(parseInsightParams({ window: "72" }).windowHours).toBe(72);
    expect(parseInsightParams({ window: "168" }).windowHours).toBe(168);
  });

  // Scenario: Window outside the allowlist falls back to the default
  it.each(["9999", "0", "-24", "abc", "", "25", "168.5"])(
    "clamps window %j to 24",
    (v) => {
      expect(parseInsightParams({ window: v }).windowHours).toBe(24);
    },
  );

  // Scenario: Span selection is sent as weeks
  it("accepts 4 and 26 for weeks", () => {
    expect(parseInsightParams({ weeks: "4" }).weeks).toBe(4);
    expect(parseInsightParams({ weeks: "26" }).weeks).toBe(26);
  });

  // Scenario: Weeks above the upper bound is not sent
  it.each(["27", "0", "-1", "x", "100"])("clamps weeks %j to 12", (v) => {
    expect(parseInsightParams({ weeks: v }).weeks).toBe(12);
  });

  // Scenario: Range outside the allowlist falls back
  it.each(["365", "0", "8", "x"])("clamps range %j to 7", (v) => {
    expect(parseInsightParams({ range: v }).rangeDays).toBe(7);
  });

  it("accepts 14 and 30 for range", () => {
    expect(parseInsightParams({ range: "14" }).rangeDays).toBe(14);
    expect(parseInsightParams({ range: "30" }).rangeDays).toBe(30);
  });

  it("takes the first value of a repeated parameter", () => {
    expect(parseInsightParams({ window: ["72", "168"] }).windowHours).toBe(72);
  });

  // Scenario: No flag entered
  it("treats absent and empty flag as none", () => {
    expect(parseInsightParams({}).flag).toEqual({ kind: "none" });
    expect(parseInsightParams({ flag: "" }).flag).toEqual({ kind: "none" });
  });

  // Scenario: Flag key at the length boundary
  it("accepts a 64 character flag", () => {
    const key = "a".repeat(64);
    expect(parseInsightParams({ flag: key }).flag).toEqual({
      kind: "valid",
      key,
    });
  });

  // Scenario: Malformed flag key is rejected (parsing side)
  it.each([
    "a".repeat(65),
    "has space",
    "<script>",
    "UPPER",
    "_leading",
    "-leading",
    ".leading",
    "a/b",
  ])("rejects malformed flag %j", (v) => {
    expect(parseInsightParams({ flag: v }).flag).toEqual({ kind: "invalid" });
  });

  it("accepts the allowed punctuation in a flag", () => {
    expect(parseInsightParams({ flag: "home_feed.ranker-v2" }).flag).toEqual({
      kind: "valid",
      key: "home_feed.ranker-v2",
    });
  });
});

describe("fetchers send the selected parameters", () => {
  // Scenario: Window selection is sent as hours
  it("sends window_hours from the selector", async () => {
    const a = stub();
    const p = parseInsightParams({ window: "168" });
    await fetchInsights(p);
    expect(a.getRecommendationPerformance.mock.calls[0][0]).toMatchObject({
      windowHours: 168,
    });
    expect(a.getSearchPerformance.mock.calls[0][0]).toMatchObject({
      windowHours: 168,
    });
  });

  // Scenario: Window outside the allowlist falls back to the default
  it("clamps unknown window to 24", async () => {
    const a = stub();
    await fetchInsights(parseInsightParams({ window: "9999" }));
    for (const call of [
      ...a.getRecommendationPerformance.mock.calls,
      ...a.getSearchPerformance.mock.calls,
    ]) {
      expect(call[0].windowHours).toBe(24);
      expect(call[0].windowHours).toBeLessThanOrEqual(168);
    }
    expect(a.getRecommendationPerformance).toHaveBeenCalledTimes(1);
    expect(a.getSearchPerformance).toHaveBeenCalledTimes(1);
  });

  // Scenario: Span selection is sent as weeks
  it("sends weeks from the selector", async () => {
    const a = stub();
    await fetchInsights(parseInsightParams({ weeks: "26" }));
    expect(a.getRetentionCohorts.mock.calls[0][0]).toMatchObject({ weeks: 26 });
  });

  // Scenario: Weeks above the upper bound is not sent
  it("clamps unknown weeks to 12", async () => {
    const a = stub();
    await fetchInsights(parseInsightParams({ weeks: "27" }));
    expect(a.getRetentionCohorts.mock.calls[0][0]).toMatchObject({ weeks: 12 });
  });

  // Scenario: Range is sent as a time window (fixed clock)
  it("range becomes from and to", async () => {
    const a = stub();
    await fetchInsights(
      parseInsightParams({ range: "30", flag: "home_feed_ranker" }),
    );
    for (const m of [a.getAiSurfaceReport, a.getExperimentReadout]) {
      const req = m.mock.calls[0][0];
      expect(req.to.toDate().getTime()).toBe(NOW.getTime());
      expect(req.from.toDate().getTime()).toBe(NOW.getTime() - 30 * DAY);
    }
    expect(a.getExperimentReadout.mock.calls[0][0].flag).toBe(
      "home_feed_ranker",
    );
  });

  // Scenario: Range outside the allowlist falls back
  it("clamps unknown range to 7", async () => {
    const a = stub();
    await fetchInsights(parseInsightParams({ range: "365" }));
    const req = a.getAiSurfaceReport.mock.calls[0][0];
    expect(req.to.toDate().getTime() - req.from.toDate().getTime()).toBe(
      7 * DAY,
    );
  });

  // Scenario: No flag entered
  it("makes no call without a flag", async () => {
    const a = stub();
    const res = await fetchInsights(defaults());
    expect(a.getExperimentReadout).not.toHaveBeenCalled();
    expect(res.experiment).toEqual({ status: "no_flag" });
    expect(a.getAiSurfaceReport).toHaveBeenCalledTimes(1);
  });

  // Scenario: Malformed flag key is rejected (no upstream call)
  it("makes no call for a malformed flag", async () => {
    const a = stub();
    const res = await fetchInsights(
      parseInsightParams({ flag: "a".repeat(65) }),
    );
    expect(a.getExperimentReadout).not.toHaveBeenCalled();
    expect(res.experiment).toEqual({ status: "bad_flag" });
  });

  // Scenario: Flag key at the length boundary
  it("accepts a 64 character flag", async () => {
    const a = stub();
    const key = "a".repeat(64);
    await fetchInsights(parseInsightParams({ flag: key }));
    expect(a.getExperimentReadout).toHaveBeenCalledTimes(1);
    expect(a.getExperimentReadout.mock.calls[0][0].flag).toBe(key);
  });

  it("uses the request bearer and a 5 s timeout on every call", async () => {
    const a = stub();
    await fetchInsights(parseInsightParams({ flag: "f" }));
    expect(makeClients).toHaveBeenCalledWith("test-token");
    for (const m of Object.values(a)) {
      expect(m).toHaveBeenCalledTimes(1);
      expect(m.mock.calls[0][1]).toMatchObject({ timeoutMs: 5000 });
    }
  });

  it("starts all five calls before any resolves", async () => {
    const pending = () => new Promise(() => {});
    const a = stub({
      getRecommendationPerformance: vi.fn(pending),
      getSearchPerformance: vi.fn(pending),
      getRetentionCohorts: vi.fn(pending),
      getExperimentReadout: vi.fn(pending),
      getAiSurfaceReport: vi.fn(pending),
    });
    void fetchInsights(parseInsightParams({ flag: "f" }));
    await Promise.resolve();
    await Promise.resolve();
    for (const m of Object.values(a)) expect(m).toHaveBeenCalledTimes(1);
  });
});

describe("result union", () => {
  it("returns ok with the response data", async () => {
    stub();
    const r = await fetchRetentionCohorts(12);
    expect(r).toEqual({
      status: "ok",
      data: { cohorts: [{ cohortWeek: "2026-11-02" }] },
    });
  });

  it("returns empty when a response has zero rows", async () => {
    stub({
      getRecommendationPerformance: vi
        .fn()
        .mockResolvedValue({ rows: [], fallback: [] }),
      getSearchPerformance: vi.fn().mockResolvedValue({ rows: [] }),
      getRetentionCohorts: vi.fn().mockResolvedValue({ cohorts: [] }),
      getExperimentReadout: vi
        .fn()
        .mockResolvedValue({ variants: [], windowDays: 7 }),
      getAiSurfaceReport: vi.fn().mockResolvedValue({ rows: [] }),
    });
    expect(await fetchRecommendationPerformance(24)).toEqual({
      status: "empty",
    });
    expect(await fetchSearchPerformance(24)).toEqual({ status: "empty" });
    expect(await fetchRetentionCohorts(12)).toEqual({ status: "empty" });
    expect(await fetchExperimentReadout("f", 7)).toEqual({ status: "empty" });
    expect(await fetchAiSurfaceReport(7)).toEqual({ status: "empty" });
  });
});

describe("error mapping", () => {
  const SECRET = "dial tcp 10.1.2.3:9090: connection refused";
  const fetchers: Array<[string, string, () => Promise<unknown>]> = [
    [
      "recommendation",
      "getRecommendationPerformance",
      () => fetchRecommendationPerformance(24),
    ],
    ["search", "getSearchPerformance", () => fetchSearchPerformance(24)],
    ["retention", "getRetentionCohorts", () => fetchRetentionCohorts(12)],
    [
      "experiment",
      "getExperimentReadout",
      () => fetchExperimentReadout("f", 7),
    ],
    ["ai", "getAiSurfaceReport", () => fetchAiSurfaceReport(7)],
  ];

  // Scenario: Expired session is redirected
  it.each(fetchers)(
    "maps Unauthenticated to unauthenticated (%s)",
    async (_n, method, run) => {
      stub({
        [method]: vi
          .fn()
          .mockRejectedValue(new ConnectError(SECRET, Code.Unauthenticated)),
      });
      expect(await run()).toEqual({ status: "unauthenticated" });
    },
  );

  it.each(fetchers)(
    "maps PermissionDenied to forbidden (%s)",
    async (_n, method, run) => {
      stub({
        [method]: vi
          .fn()
          .mockRejectedValue(new ConnectError(SECRET, Code.PermissionDenied)),
      });
      expect(await run()).toEqual({ status: "forbidden" });
    },
  );

  // Scenario: Upstream rejects the request
  it.each(fetchers)(
    "maps InvalidArgument to invalid without a message (%s)",
    async (_n, method, run) => {
      stub({
        [method]: vi
          .fn()
          .mockRejectedValue(new ConnectError(SECRET, Code.InvalidArgument)),
      });
      const r = await run();
      expect(r).toEqual({ status: "invalid" });
      expect(JSON.stringify(r)).not.toContain("10.1.2.3");
    },
  );

  // Scenario: Report service unavailable
  it.each(fetchers)(
    "maps Unavailable, timeout and unknown errors to unavailable (%s)",
    async (_n, method, run) => {
      for (const err of [
        new ConnectError(SECRET, Code.Unavailable),
        new ConnectError(SECRET, Code.DeadlineExceeded),
        new ConnectError(SECRET, Code.Internal),
        new Error(SECRET),
      ]) {
        stub({ [method]: vi.fn().mockRejectedValue(err) });
        const r = await run();
        expect(r).toEqual({ status: "unavailable" });
        expect(JSON.stringify(r)).not.toContain("10.1.2.3");
      }
    },
  );

  it("one failing fetch does not change the others", async () => {
    stub({
      getRetentionCohorts: vi
        .fn()
        .mockRejectedValue(new ConnectError(SECRET, Code.Unavailable)),
    });
    const res = await fetchInsights(parseInsightParams({ flag: "f" }));
    expect(res.retention).toEqual({ status: "unavailable" });
    expect(res.recommendation.status).toBe("ok");
    expect(res.search.status).toBe("ok");
    expect(res.experiment.status).toBe("ok");
    expect(res.aiSurfaces.status).toBe("ok");
  });
});
