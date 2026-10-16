import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createReport } from "@/lib/gateway/reports";

import { reportListingAction } from "./reportActions";

vi.mock("@/lib/gateway/reports", () => ({
  createReport: vi.fn(),
  listReports: vi.fn(),
  resolveReport: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
  hasScope: vi.fn(),
}));

// Contract under test (implementer creates src/features/listing/reportActions.ts):
//   "use server"
//   reportListingAction(listingId, reason, note)
//     => { ok: true; message } | { ok: false; error }
//   Trims reason and note; reason required, note 0 to 500; calls gateway
//   createReport(listingId, reason, note) at most once. An unauthenticated
//   result is returned as an error message (the dialog stays open), not a redirect.

const REPORT_VIEW = {
  id: "r1",
  listingId: "l1",
  reasonCode: "scam",
  note: "",
  reporterId: "u-1",
  status: "open" as const,
  createdAt: "2026-10-01T00:00:00.000Z",
  resolutionNote: "",
};
const SUCCESS = "Đã gửi báo cáo. Cảm ơn bạn.";
const OWN = "Bạn không thể báo cáo tin đăng của chính mình.";
const PERM = "Bạn không có quyền thực hiện thao tác này.";
const CAP =
  "Bạn đang có quá nhiều báo cáo chờ xử lý. Vui lòng thử lại sau khi các báo cáo được xử lý.";
const EXPIRED = "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.";
const DOWN = "Chưa gửi được báo cáo. Vui lòng thử lại sau.";

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(createReport).mockResolvedValue({
    status: "ok",
    report: REPORT_VIEW,
  });
});
afterEach(() => vi.restoreAllMocks());

describe("reportListingAction", () => {
  // Scenario: Moderation UI: Submit a report with reason and note
  it("sends one CreateReport call", async () => {
    const res = await reportListingAction(
      "l1",
      "scam",
      "Giá quá rẻ, yêu cầu chuyển khoản ngoài",
    );
    expect(res).toEqual({ ok: true, message: SUCCESS });
    expect(createReport).toHaveBeenCalledTimes(1);
    expect(createReport).toHaveBeenCalledWith(
      "l1",
      "scam",
      "Giá quá rẻ, yêu cầu chuyển khoản ngoài",
    );
  });

  // Scenario: Moderation UI: Submit a report with reason and note
  it("trims reason and note and allows an empty note", async () => {
    await reportListingAction("l1", "  scam ", "  ghi chú \n");
    expect(createReport).toHaveBeenCalledWith("l1", "scam", "ghi chú");

    vi.mocked(createReport).mockClear();
    const res = await reportListingAction("l1", "other", "");
    expect(res.ok).toBe(true);
    expect(createReport).toHaveBeenCalledWith("l1", "other", "");
  });

  // Scenario: Moderation UI: Reason is required
  it.each(["", "   "])("blocks reason %j locally", async (reason) => {
    const res = await reportListingAction("l1", reason, "x");
    expect(res).toEqual({ ok: false, error: "Vui lòng chọn lý do." });
    expect(createReport).not.toHaveBeenCalled();
  });

  // Scenario: Moderation UI: Report note length boundary
  it("validates note length up to 500", async () => {
    const ok = await reportListingAction("l1", "scam", "a".repeat(500));
    expect(ok.ok).toBe(true);
    expect(createReport).toHaveBeenCalledTimes(1);

    vi.mocked(createReport).mockClear();
    const bad = await reportListingAction("l1", "scam", "a".repeat(501));
    expect(bad.ok).toBe(false);
    expect(createReport).not.toHaveBeenCalled();
  });

  // Scenario: Moderation UI: Report note length boundary
  it("measures the note after trimming", async () => {
    const res = await reportListingAction(
      "l1",
      "scam",
      `  ${"a".repeat(500)}  `,
    );
    expect(res.ok).toBe(true);
    expect(createReport).toHaveBeenCalledWith("l1", "scam", "a".repeat(500));
  });

  // Scenario: Moderation UI: Reporting twice is quietly accepted
  it("an existing open report is a success", async () => {
    vi.mocked(createReport).mockResolvedValue({
      status: "ok",
      report: { ...REPORT_VIEW, id: "r-existing", note: "lần đầu" },
    });
    const res = await reportListingAction("l1", "scam", "lần hai");
    expect(res).toEqual({ ok: true, message: SUCCESS });
  });

  // Scenario: Moderation UI: Seller reporting their own listing is refused
  it("maps own-listing refusal", async () => {
    vi.mocked(createReport).mockResolvedValue({ status: "failed_precondition" });
    expect(await reportListingAction("l1", "scam", "n")).toEqual({
      ok: false,
      error: OWN,
    });
  });

  // Scenario: Moderation UI: Reporter without permission is refused
  it("maps permission denied apart from own listing", async () => {
    vi.mocked(createReport).mockResolvedValue({ status: "forbidden" });
    const res = await reportListingAction("l1", "scam", "n");
    expect(res).toEqual({ ok: false, error: PERM });
    expect(res.ok === false && res.error).not.toBe(OWN);
  });

  // Scenario: Moderation UI: Open report cap is explained
  it("maps resource exhausted to the cap message", async () => {
    vi.mocked(createReport).mockResolvedValue({ status: "resource_exhausted" });
    expect(await reportListingAction("l1", "scam", "n")).toEqual({
      ok: false,
      error: CAP,
    });
  });

  // Scenario: Moderation UI: Expired session while reporting
  it("maps unauthenticated", async () => {
    vi.mocked(createReport).mockResolvedValue({ status: "unauthenticated" });
    expect(await reportListingAction("l1", "scam", "n")).toEqual({
      ok: false,
      error: EXPIRED,
    });
  });

  // Scenario: Moderation UI: Report backend unavailable
  it("maps unavailable and unknown errors", async () => {
    vi.mocked(createReport).mockResolvedValue({ status: "unavailable" });
    expect(await reportListingAction("l1", "scam", "n")).toEqual({
      ok: false,
      error: DOWN,
    });

    vi.mocked(createReport).mockResolvedValue({ status: "not_found" });
    expect((await reportListingAction("l1", "scam", "n")).ok).toBe(false);

    vi.mocked(createReport).mockRejectedValue(new Error("pg-1:5432 down"));
    const res = await reportListingAction("l1", "scam", "n");
    expect(res).toEqual({ ok: false, error: DOWN });
    expect(JSON.stringify(res)).not.toContain("pg-1");
  });

  // Scenario: Moderation UI: Report backend unavailable
  it("never logs the note", async () => {
    const spies = [
      vi.spyOn(console, "log").mockImplementation(() => {}),
      vi.spyOn(console, "error").mockImplementation(() => {}),
      vi.spyOn(console, "warn").mockImplementation(() => {}),
      vi.spyOn(console, "info").mockImplementation(() => {}),
    ];
    vi.mocked(createReport).mockRejectedValue(new Error("down"));
    await reportListingAction("l1", "scam", "BÍ-MẬT-NOTE");
    for (const s of spies) {
      expect(JSON.stringify(s.mock.calls)).not.toContain("BÍ-MẬT-NOTE");
    }
  });
});
