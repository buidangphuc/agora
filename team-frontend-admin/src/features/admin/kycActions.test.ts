import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { getKycDocumentUrl, reviewKyc } from "@/lib/gateway/kyc";
import { getPrincipal } from "@/lib/gateway/session";

import { reviewKycAction, viewKycDocumentAction } from "./kycActions";

vi.mock("@/lib/gateway/kyc", () => ({
  listKycSubmissions: vi.fn(),
  getKycDocumentUrl: vi.fn(),
  reviewKyc: vi.fn(),
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

// Contract under test (implementer creates src/features/admin/kycActions.ts, "use server"):
//   viewKycDocumentAction(formData: FormData) with fields submissionId, index (decimal string)
//     => { ok: true; url: string; expiresAt: string } | { ok: false; error: string }
//   reviewKycAction(formData: FormData) with fields submissionId, decision ("approve"|"reject"), reason
//     => { ok: true; message: string } | { ok: false; error: string }
//   Both: no principal => redirectGuestToLogin("/admin/cases"); no "admin" scope => forbidden message.
//   Success of review calls revalidatePath("/admin/cases"). Approve sends reasonCode "".

const NO_PERM = "Bạn không có quyền thực hiện thao tác này.";
const BAD_INPUT = "Dữ liệu không hợp lệ; hồ sơ không thay đổi.";
const NOT_FOUND_DOC = "Không tìm thấy tài liệu này.";
const AUDIT_DOWN =
  "Không ghi được nhật ký truy cập nên chưa mở được tài liệu. Vui lòng thử lại sau.";
const SELF =
  "Bạn không thể xử lý hồ sơ của chính mình hoặc không có quyền thực hiện thao tác này.";
const DECIDED = "Hồ sơ đã được xử lý hoặc không còn ở trạng thái chờ.";
const REVIEW_DOWN = "Không xử lý được hồ sơ. Vui lòng thử lại sau.";

function form(fields: Record<string, string>): FormData {
  const fd = new FormData();
  for (const [k, v] of Object.entries(fields)) fd.set(k, v);
  return fd;
}
const viewForm = (f: Record<string, string> = {}) =>
  form({ submissionId: "k-1", index: "0", ...f });
const reviewForm = (f: Record<string, string> = {}) =>
  form({ submissionId: "k-1", decision: "approve", reason: "", ...f });

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "admin-1",
    name: "root",
    scopes: ["admin"],
  });
  vi.mocked(getKycDocumentUrl).mockResolvedValue({
    status: "ok",
    url: "https://files.example/doc?sig=abc",
    expiresAt: "2026-11-15T08:05:00.000Z",
  } as never);
  vi.mocked(reviewKyc).mockResolvedValue({ status: "ok" });
});

describe("viewKycDocumentAction", () => {
  it("KYC queue: A non-admin cannot obtain a document link - viewKycDocumentAction refuses a non-admin", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-1",
      name: "buyer",
      scopes: ["buyer"],
    });
    const res = await viewKycDocumentAction(viewForm());
    expect(res).toEqual({ ok: false, error: NO_PERM });
    expect(getKycDocumentUrl).not.toHaveBeenCalled();
  });

  it("KYC queue: A non-admin cannot obtain a document link - guest is redirected to login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null as never);
    await expect(viewKycDocumentAction(viewForm())).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirectGuestToLogin).toHaveBeenCalledWith("/admin/cases");
    expect(getKycDocumentUrl).not.toHaveBeenCalled();
  });

  it("KYC queue: Viewing a document reveals a 5-minute link - passes id and index and returns url with expiry", async () => {
    const res = await viewKycDocumentAction(
      viewForm({ submissionId: "k-9", index: "2" }),
    );
    expect(res).toEqual({
      ok: true,
      url: "https://files.example/doc?sig=abc",
      expiresAt: "2026-11-15T08:05:00.000Z",
    });
    expect(getKycDocumentUrl).toHaveBeenCalledTimes(1);
    expect(getKycDocumentUrl).toHaveBeenCalledWith("k-9", 2);
  });

  it.each([
    ["empty id", { submissionId: "" }],
    ["negative index", { index: "-1" }],
    ["fractional index", { index: "1.5" }],
    ["non-numeric index", { index: "abc" }],
    ["empty index", { index: "" }],
  ])(
    "KYC queue: Unknown submission or index is refused - malformed input (%s) never reaches the gateway",
    async (_n, f) => {
      const res = await viewKycDocumentAction(viewForm(f));
      expect(res.ok).toBe(false);
      expect(getKycDocumentUrl).not.toHaveBeenCalled();
    },
  );

  it.each(["not_found", "invalid"])(
    "KYC queue: Unknown submission or index is refused - viewKycDocumentAction maps not_found and invalid to the not found message (%s)",
    async (status) => {
      vi.mocked(getKycDocumentUrl).mockResolvedValue({ status } as never);
      const res = await viewKycDocumentAction(viewForm());
      expect(res).toEqual({ ok: false, error: NOT_FOUND_DOC });
    },
  );

  it("KYC queue: Audit store down fails closed - unavailable gives the audit message and no url", async () => {
    vi.mocked(getKycDocumentUrl).mockResolvedValue({
      status: "unavailable",
    } as never);
    const res = await viewKycDocumentAction(viewForm());
    expect(res).toEqual({ ok: false, error: AUDIT_DOWN });
    expect(JSON.stringify(res)).not.toContain("http");
  });

  it("KYC queue: Gateway refusal - forbidden gives the no-permission message", async () => {
    vi.mocked(getKycDocumentUrl).mockResolvedValue({
      status: "forbidden",
    } as never);
    const res = await viewKycDocumentAction(viewForm());
    expect(res).toEqual({ ok: false, error: NO_PERM });
  });

  it("KYC queue: Expired session is sent to login - unauthenticated redirects", async () => {
    vi.mocked(getKycDocumentUrl).mockResolvedValue({
      status: "unauthenticated",
    } as never);
    await expect(viewKycDocumentAction(viewForm())).rejects.toThrow(
      "NEXT_REDIRECT",
    );
  });
});

describe("reviewKycAction", () => {
  it("KYC queue: A non-admin cannot review - reviewKycAction refuses a non-admin", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "s-1",
      name: "seller",
      scopes: ["seller"],
    });
    const res = await reviewKycAction(reviewForm());
    expect(res).toEqual({ ok: false, error: NO_PERM });
    expect(reviewKyc).not.toHaveBeenCalled();
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("KYC queue: A non-admin cannot review - guest is redirected to login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null as never);
    await expect(reviewKycAction(reviewForm())).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(reviewKyc).not.toHaveBeenCalled();
  });

  it.each([
    ["decision ban", { decision: "ban" }],
    ["decision uppercase", { decision: "APPROVE" }],
    ["empty decision", { decision: "" }],
    [
      "reason with bad characters",
      { decision: "reject", reason: "Bad Reason!" },
    ],
    [
      "reason not in the fixed list",
      { decision: "reject", reason: "made_up_reason" },
    ],
    ["reject without reason", { decision: "reject", reason: "" }],
    ["empty id", { submissionId: "" }],
  ])(
    "KYC queue: Forged decision or reason is refused by the action - reviewKycAction refuses a forged decision or reason code (%s)",
    async (_n, f) => {
      const res = await reviewKycAction(reviewForm(f));
      expect(res).toEqual({ ok: false, error: BAD_INPUT });
      expect(reviewKyc).not.toHaveBeenCalled();
      expect(revalidatePath).not.toHaveBeenCalled();
    },
  );

  it("KYC queue: Approve removes the row and verifies the seller - approve sends empty reason, toasts and revalidates", async () => {
    const res = await reviewKycAction(reviewForm({ reason: "other" }));
    expect(res).toEqual({ ok: true, message: "Đã duyệt hồ sơ xác minh." });
    expect(reviewKyc).toHaveBeenCalledTimes(1);
    expect(reviewKyc).toHaveBeenCalledWith("k-1", "approve", "");
    expect(revalidatePath).toHaveBeenCalledTimes(1);
    expect(revalidatePath).toHaveBeenCalledWith("/admin/cases");
  });

  it("KYC queue: Reject with a reason code - reject sends the reason, toasts and revalidates", async () => {
    const res = await reviewKycAction(
      reviewForm({ decision: "reject", reason: "document_unreadable" }),
    );
    expect(res).toEqual({ ok: true, message: "Đã từ chối hồ sơ xác minh." });
    expect(reviewKyc).toHaveBeenCalledWith(
      "k-1",
      "reject",
      "document_unreadable",
    );
    expect(revalidatePath).toHaveBeenCalledWith("/admin/cases");
  });

  it("KYC queue: Reviewing your own submission is refused - forbidden from the service shows the self-review message and no refresh", async () => {
    vi.mocked(reviewKyc).mockResolvedValue({ status: "forbidden" });
    const res = await reviewKycAction(reviewForm());
    expect(res).toEqual({ ok: false, error: SELF });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("KYC queue: Second decision on the same submission fails - failed_precondition shows the already-decided message", async () => {
    vi.mocked(reviewKyc).mockResolvedValue({ status: "failed_precondition" });
    const res = await reviewKycAction(reviewForm());
    expect(res).toEqual({ ok: false, error: DECIDED });
  });

  it.each(["unavailable", "not_found", "invalid"] as const)(
    "KYC queue: Review failure leaves the row unchanged - %s shows a retry or refusal message and does not revalidate",
    async (status) => {
      vi.mocked(reviewKyc).mockResolvedValue({ status });
      const res = await reviewKycAction(reviewForm());
      expect(res.ok).toBe(false);
      if (status === "unavailable") {
        expect(res).toEqual({ ok: false, error: REVIEW_DOWN });
      }
      expect(revalidatePath).not.toHaveBeenCalled();
    },
  );

  it("KYC queue: Expired session is sent to login - unauthenticated redirects", async () => {
    vi.mocked(reviewKyc).mockResolvedValue({ status: "unauthenticated" });
    await expect(reviewKycAction(reviewForm())).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});
