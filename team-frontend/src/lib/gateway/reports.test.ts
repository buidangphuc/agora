import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  ReportStatus,
  ReportTargetType,
} from "@/generated/platform/engagement/v1/engagement_pb.js";

import { makeClients } from "./client.js";
import { createReport, listReports, resolveReport } from "./reports.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

// Contract under test (implementer creates src/lib/gateway/reports.ts):
//   createReport(listingId, reasonCode, note) =>
//     { status: "ok"; report: ReportView } | { status: ErrStatus }
//   listReports(status: "open" | "actioned" | "dismissed", cursor) =>
//     { status: "ok"; items: ReportView[]; nextCursor: string } | { status: ErrStatus }
//   resolveReport(reportId, outcome: "actioned" | "dismissed", note) =>
//     { status: "ok" } | { status: ErrStatus }
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

describe("listReports gateway wrapper", () => {
  // Scenario: Moderation UI: Default tab is reports with status OPEN
  it.each([
    ["open", ReportStatus.OPEN],
    ["actioned", ReportStatus.ACTIONED],
    ["dismissed", ReportStatus.DISMISSED],
  ] as const)("status %s sends page size 20 and maps rows", async (s, enumV) => {
    const engagement = stub({
      listReports: vi.fn().mockResolvedValue({
        reports: [report("r1", enumV), report("r2", enumV)],
        page: { nextCursor: "20" },
      }),
    });
    const res = await listReports(s, "");
    const [req, opts] = engagement.listReports.mock.calls[0];
    expect(req).toMatchObject({ status: enumV });
    expect(req.page).toMatchObject({ cursor: "", pageSize: 20 });
    expect(opts).toMatchObject({ timeoutMs: 5000 });
    expect(res.status).toBe("ok");
    if (res.status !== "ok") return;
    expect(res.items.map((i) => i.id)).toEqual(["r1", "r2"]);
    expect(res.items[0]).toMatchObject({
      listingId: "l-r1",
      reasonCode: "scam",
      note: "Giá quá rẻ",
      reporterId: "u-7",
      status: s,
      createdAt: "2026-10-01T00:00:00.000Z",
    });
    expect(res.nextCursor).toBe("20");
  });

  // Scenario: Moderation UI: Empty report list
  it("forwards the cursor and returns an empty next cursor on the last page", async () => {
    const engagement = stub({
      listReports: vi.fn().mockResolvedValue({ reports: [], page: {} }),
    });
    const res = await listReports("open", "40");
    expect(engagement.listReports.mock.calls[0][0].page).toMatchObject({
      cursor: "40",
      pageSize: 20,
    });
    expect(res).toMatchObject({ status: "ok", items: [], nextCursor: "" });
  });

  // Scenarios: Reports backend unavailable; Expired session is sent to login;
  // Gateway refusal is shown as the same 403
  it.each(ERR_CODES)("maps code %s to %s", async (code, status) => {
    stub({
      listReports: vi
        .fn()
        .mockRejectedValue(new ConnectError("10.0.0.7:5432", code)),
    });
    expect(await listReports("open", "")).toEqual({ status });
  });
});

describe("resolveReport gateway wrapper", () => {
  // Scenario: Moderation UI: Close a report as actioned
  it("actioned sends one ResolveReport call with ACTIONED and the note", async () => {
    const engagement = stub({
      resolveReport: vi.fn().mockResolvedValue({ report: report("r1") }),
    });
    const res = await resolveReport("r1", "actioned", "Đã khóa tin đăng");
    expect(res).toEqual({ status: "ok" });
    expect(engagement.resolveReport).toHaveBeenCalledTimes(1);
    const [req, opts] = engagement.resolveReport.mock.calls[0];
    expect(req).toMatchObject({
      reportId: "r1",
      status: ReportStatus.ACTIONED,
      resolutionNote: "Đã khóa tin đăng",
    });
    expect(opts).toMatchObject({ timeoutMs: 5000 });
  });

  // Scenario: Moderation UI: Dismiss a report
  it("dismissed sends DISMISSED", async () => {
    const engagement = stub({
      resolveReport: vi.fn().mockResolvedValue({ report: report("r1") }),
    });
    await resolveReport("r1", "dismissed", "Không vi phạm");
    expect(engagement.resolveReport.mock.calls[0][0]).toMatchObject({
      status: ReportStatus.DISMISSED,
    });
  });

  // Scenarios: Close fails closed when audit is down; Already-closed report;
  // A forged close by a non-admin is refused
  it.each(ERR_CODES)(
    "maps code %s to %s and never leaks err.message",
    async (code, status) => {
      stub({
        resolveReport: vi
          .fn()
          .mockRejectedValue(new ConnectError("pg-1:5432 secret", code)),
      });
      const res = await resolveReport("r1", "actioned", "n");
      expect(res).toEqual({ status });
      expect(JSON.stringify(res)).not.toContain("pg-1");
    },
  );

  // Scenario: Moderation UI: Close fails closed when audit is down
  it("maps a non-Connect failure to unavailable", async () => {
    stub({ resolveReport: vi.fn().mockRejectedValue(new Error("boom")) });
    expect(await resolveReport("r1", "actioned", "n")).toEqual({
      status: "unavailable",
    });
  });
});
