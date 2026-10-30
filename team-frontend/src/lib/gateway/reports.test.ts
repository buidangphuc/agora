import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  ReportStatus,
  ReportTargetType,
} from "@/generated/platform/engagement/v1/engagement_pb.js";

import { makeClients } from "./client.js";
import { createReport } from "./reports.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

// Contract under test (implementer creates src/lib/gateway/reports.ts):
//   createReport(listingId, reasonCode, note) =>
//     { status: "ok"; report: ReportView } | { status: ErrStatus }
//   ErrStatus = "unauthenticated" | "forbidden" | "not_found" |
//     "failed_precondition" | "invalid" | "resource_exhausted" | "unavailable"
//   ReportView = { id, listingId, reasonCode, note, reporterId: string;
//     status: "open" | "actioned" | "dismissed";
//     createdAt: string /* ISO-8601 UTC */; resolutionNote: string }
//   Results never carry err.message. All calls use a 5 s timeout.

function stub(engagement: Record<string, ReturnType<typeof vi.fn>>) {
  vi.mocked(makeClients).mockReturnValue({ engagement } as never);
  return engagement;
}

function report(id: string, status = ReportStatus.OPEN) {
  return {
    id,
    targetType: ReportTargetType.LISTING,
    targetId: `l-${id}`,
    reasonCode: "scam",
    note: "Giá quá rẻ",
    reporterId: "u-7",
    status,
    createdAt: Timestamp.fromDate(new Date("2026-10-01T00:00:00Z")),
    resolutionNote: "",
  };
}

const ERR_CODES = [
  [Code.Unauthenticated, "unauthenticated"],
  [Code.PermissionDenied, "forbidden"],
  [Code.NotFound, "not_found"],
  [Code.FailedPrecondition, "failed_precondition"],
  [Code.InvalidArgument, "invalid"],
  [Code.ResourceExhausted, "resource_exhausted"],
  [Code.Unavailable, "unavailable"],
  [Code.DeadlineExceeded, "unavailable"],
] as const;

beforeEach(() => vi.clearAllMocks());

describe("createReport gateway wrapper", () => {
  // Scenario: Moderation UI: Submit a report with reason and note
  it("sends one CreateReport call with target type LISTING", async () => {
    const engagement = stub({
      createReport: vi.fn().mockResolvedValue({ report: report("r1") }),
    });
    const res = await createReport("l1", "scam", "Giá quá rẻ");
    expect(engagement.createReport).toHaveBeenCalledTimes(1);
    const [req, opts] = engagement.createReport.mock.calls[0];
    expect(req).toMatchObject({
      targetType: ReportTargetType.LISTING,
      targetId: "l1",
      reasonCode: "scam",
      note: "Giá quá rẻ",
    });
    expect(opts).toMatchObject({ timeoutMs: 5000 });
    expect(res.status).toBe("ok");
  });

  // Scenarios: Own listing refused; Reporter without permission; Open report
  // cap; Expired session while reporting; Report backend unavailable
  it.each(ERR_CODES)(
    "maps code %s to %s and never leaks err.message",
    async (code, status) => {
      stub({
        createReport: vi
          .fn()
          .mockRejectedValue(new ConnectError("pg-1:5432 secret", code)),
      });
      const res = await createReport("l1", "scam", "");
      expect(res).toEqual({ status });
      expect(JSON.stringify(res)).not.toContain("pg-1");
    },
  );

  // Scenario: Moderation UI: Report backend unavailable
  it("maps a non-Connect failure to unavailable", async () => {
    stub({ createReport: vi.fn().mockRejectedValue(new Error("boom")) });
    expect(await createReport("l1", "scam", "")).toEqual({
      status: "unavailable",
    });
  });
});
