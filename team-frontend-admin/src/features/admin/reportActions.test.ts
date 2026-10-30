import { revalidatePath } from "next/cache";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { resolveReport } from "@/lib/gateway/reports";
import { getPrincipal } from "@/lib/gateway/session";

import { closeReportAction } from "./reportActions";

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
vi.mock("@/features/auth/guardPath", () => ({
  redirectGuestToLogin: vi.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
}));

// Contract under test (implementer creates src/features/admin/reportActions.ts):
//   "use server"
//   closeReportAction(reportId, outcome: "actioned" | "dismissed", note)
//     => { ok: true; message } | { ok: false; error }
//   Trims note (1 to 500 chars); calls gateway resolveReport(reportId,
//   outcome, note) at most once; no role argument.

const PERM = "Bạn không có quyền thực hiện thao tác này.";
const AUDIT =
  "Chưa ghi được nhật ký kiểm toán nên thao tác chưa được thực hiện. Vui lòng thử lại sau.";

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "admin-1",
    name: "root",
    scopes: ["admin"],
  });
  vi.mocked(resolveReport).mockResolvedValue({ status: "ok" });
});
afterEach(() => vi.restoreAllMocks());

describe("closeReportAction", () => {
  // Scenario: Moderation UI: Close a report as actioned
  it("close sends one call and revalidates", async () => {
    const res = await closeReportAction("r1", "actioned", "Đã khóa tin đăng");
    expect(res).toEqual({ ok: true, message: "Đã đóng báo cáo." });
    expect(resolveReport).toHaveBeenCalledTimes(1);
    expect(resolveReport).toHaveBeenCalledWith(
      "r1",
      "actioned",
      "Đã khóa tin đăng",
    );
    expect(revalidatePath).toHaveBeenCalledWith("/admin/cases");
  });

  // Scenario: Moderation UI: Dismiss a report
  it("dismiss sends DISMISSED", async () => {
    const res = await closeReportAction("r1", "dismissed", "Không vi phạm");
    expect(res).toEqual({ ok: true, message: "Đã đóng báo cáo." });
    expect(resolveReport).toHaveBeenCalledWith(
      "r1",
      "dismissed",
      "Không vi phạm",
    );
  });

  // Scenario: Moderation UI: Close a report as actioned
  it("trims the note before sending", async () => {
    await closeReportAction("r1", "actioned", "  xong \n");
    expect(resolveReport).toHaveBeenCalledWith("r1", "actioned", "xong");
  });

  // Scenario: Moderation UI: Close note length boundary
  it("validates note length 1 to 500", async () => {
    const ok = await closeReportAction("r1", "actioned", "a".repeat(500));
    expect(ok.ok).toBe(true);
    expect(resolveReport).toHaveBeenCalledTimes(1);

    vi.mocked(resolveReport).mockClear();
    const bad = await closeReportAction("r1", "actioned", "a".repeat(501));
    expect(bad.ok).toBe(false);
    expect(resolveReport).not.toHaveBeenCalled();
  });

  // Scenario: Moderation UI: Close note length boundary
  it("measures the note after trimming", async () => {
    const res = await closeReportAction(
      "r1",
      "actioned",
      `  ${"a".repeat(500)}  `,
    );
    expect(res.ok).toBe(true);
    expect(resolveReport).toHaveBeenCalledWith(
      "r1",
      "actioned",
      "a".repeat(500),
    );
  });

  // Scenario: Moderation UI: Close note length boundary
  it.each(["", "   ", "\n\t"])("blocks an empty note %j locally", async (n) => {
    const res = await closeReportAction("r1", "actioned", n);
    expect(res).toEqual({ ok: false, error: "Vui lòng nhập ghi chú xử lý." });
    expect(resolveReport).not.toHaveBeenCalled();
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  // Scenario: Moderation UI: Close fails closed when audit is down
  it("maps unavailable to the audit message", async () => {
    vi.mocked(resolveReport).mockResolvedValue({ status: "unavailable" });
    const res = await closeReportAction("r1", "actioned", "n");
    expect(res).toEqual({ ok: false, error: AUDIT });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  // Scenario: Moderation UI: Already-closed report
  it("maps failed precondition", async () => {
    vi.mocked(resolveReport).mockResolvedValue({
      status: "failed_precondition",
    });
    const res = await closeReportAction("r1", "actioned", "n");
    expect(res).toEqual({ ok: false, error: "Báo cáo này đã được xử lý." });
    expect(revalidatePath).toHaveBeenCalledWith("/admin/cases");
  });

  // Scenario: Moderation UI: A forged close by a non-admin is refused
  it("maps permission denied", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-9",
      name: "buyer",
      scopes: [],
    });
    vi.mocked(resolveReport).mockResolvedValue({ status: "forbidden" });
    const res = await closeReportAction("r1", "actioned", "n");
    expect(res).toEqual({ ok: false, error: PERM });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  // Scenario: Moderation UI: Expired session is sent to login
  it("redirects to login when the gateway says unauthenticated", async () => {
    vi.mocked(resolveReport).mockResolvedValue({ status: "unauthenticated" });
    await expect(closeReportAction("r1", "actioned", "n")).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirectGuestToLogin).toHaveBeenCalledWith("/admin/cases");
  });

  // Scenario: Moderation UI: Close fails closed when audit is down
  it("never shows err.message and never logs the note", async () => {
    const spies = [
      vi.spyOn(console, "log").mockImplementation(() => {}),
      vi.spyOn(console, "error").mockImplementation(() => {}),
      vi.spyOn(console, "warn").mockImplementation(() => {}),
      vi.spyOn(console, "info").mockImplementation(() => {}),
    ];
    vi.mocked(resolveReport).mockRejectedValue(new Error("pg-1:5432 down"));
    const res = await closeReportAction("r1", "actioned", "BÍ-MẬT-NOTE");
    expect(res.ok).toBe(false);
    expect(JSON.stringify(res)).not.toContain("pg-1");
    for (const s of spies) {
      expect(JSON.stringify(s.mock.calls)).not.toContain("BÍ-MẬT-NOTE");
    }
  });
});
