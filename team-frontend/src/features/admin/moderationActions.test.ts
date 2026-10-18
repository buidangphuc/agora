import { revalidatePath } from "next/cache";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { moderateListing } from "@/lib/gateway/moderation";
import { getPrincipal } from "@/lib/gateway/session";

import { moderateListingAction } from "./moderationActions";

vi.mock("@/lib/gateway/moderation", () => ({
  listModerationQueue: vi.fn(),
  moderateListing: vi.fn(),
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

// Contract under test (implementer creates src/features/admin/moderationActions.ts):
//   "use server"
//   moderateListingAction(listingId, decision: "approve"|"reject"|"ban",
//                         reasonCode, note)
//     => { ok: true; message } | { ok: false; error }
//   Trims reasonCode and note; calls gateway moderateListing(listingId,
//   decision, reasonCode, note) at most once; no role argument.

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
  vi.mocked(moderateListing).mockResolvedValue({ status: "ok" });
});
afterEach(() => vi.restoreAllMocks());

describe("moderateListingAction", () => {
  // Scenario: Moderation UI: Approve publishes and removes the row
  it("approve sends one call and revalidates", async () => {
    const res = await moderateListingAction("l1", "approve", "", "");
    expect(res).toEqual({ ok: true, message: "Đã duyệt tin đăng." });
    expect(moderateListing).toHaveBeenCalledTimes(1);
    expect(moderateListing).toHaveBeenCalledWith("l1", "approve", "", "");
    expect(revalidatePath).toHaveBeenCalledWith("/admin/listings");
    expect(revalidatePath).toHaveBeenCalledWith("/admin/cases");
  });

  // Scenario: Moderation UI: Reject sends reason and note
  it("reject sends reason and note", async () => {
    const res = await moderateListingAction(
      "l1",
      "reject",
      "counterfeit",
      "Ảnh sản phẩm là hàng nhái",
    );
    expect(res).toEqual({ ok: true, message: "Đã từ chối tin đăng." });
    expect(moderateListing).toHaveBeenCalledWith(
      "l1",
      "reject",
      "counterfeit",
      "Ảnh sản phẩm là hàng nhái",
    );
  });

  // Scenario: Moderation UI: Reject sends reason and note
  it("trims reason and note before sending", async () => {
    await moderateListingAction("l1", "reject", "  other ", "  ghi chú \n");
    expect(moderateListing).toHaveBeenCalledWith(
      "l1",
      "reject",
      "other",
      "ghi chú",
    );
  });

  // Scenario: Moderation UI: Reject sends reason and note
  it.each(["reject", "ban"] as const)(
    "%s without a reason is blocked locally",
    async (d) => {
      for (const reason of ["", "   "]) {
        const res = await moderateListingAction("l1", d, reason, "");
        expect(res).toEqual({ ok: false, error: "Vui lòng chọn lý do." });
      }
      expect(moderateListing).not.toHaveBeenCalled();
      expect(revalidatePath).not.toHaveBeenCalled();
    },
  );

  // Scenario: Moderation UI: Ban removes the listing from the queue
  it("ban sends one call", async () => {
    const res = await moderateListingAction("l1", "ban", "prohibited_item", "");
    expect(res).toEqual({ ok: true, message: "Đã khóa tin đăng." });
    expect(moderateListing).toHaveBeenCalledTimes(1);
    expect(moderateListing).toHaveBeenCalledWith(
      "l1",
      "ban",
      "prohibited_item",
      "",
    );
    expect(revalidatePath).toHaveBeenCalledWith("/admin/listings");
  });

  // Scenario: Moderation UI: Moderation note length boundary
  it("accepts a 500-character note and rejects 501 locally", async () => {
    const ok = await moderateListingAction(
      "l1",
      "reject",
      "other",
      "a".repeat(500),
    );
    expect(ok.ok).toBe(true);
    expect(moderateListing).toHaveBeenCalledTimes(1);

    vi.mocked(moderateListing).mockClear();
    const bad = await moderateListingAction(
      "l1",
      "reject",
      "other",
      "a".repeat(501),
    );
    expect(bad.ok).toBe(false);
    expect(moderateListing).not.toHaveBeenCalled();
  });

  // Scenario: Moderation UI: Moderation note length boundary
  it("measures the note after trimming", async () => {
    const res = await moderateListingAction(
      "l1",
      "reject",
      "other",
      `  ${"a".repeat(500)}  `,
    );
    expect(res.ok).toBe(true);
    expect(moderateListing).toHaveBeenCalledWith(
      "l1",
      "reject",
      "other",
      "a".repeat(500),
    );
  });

  // Scenario: Moderation UI: Audit unavailable leaves the row
  it("maps unavailable to the audit message", async () => {
    vi.mocked(moderateListing).mockResolvedValue({ status: "unavailable" });
    const res = await moderateListingAction("l1", "approve", "", "");
    expect(res).toEqual({ ok: false, error: AUDIT });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  // Scenario: Moderation UI: Not-allowed transition is explained
  it("maps failed precondition", async () => {
    vi.mocked(moderateListing).mockResolvedValue({
      status: "failed_precondition",
    });
    const res = await moderateListingAction("l1", "approve", "", "");
    expect(res).toEqual({
      ok: false,
      error: "Trạng thái hiện tại của tin đăng không cho phép thao tác này.",
    });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  // Scenario: Moderation UI: Listing is gone
  it("maps not found", async () => {
    vi.mocked(moderateListing).mockResolvedValue({ status: "not_found" });
    const res = await moderateListingAction("l1", "approve", "", "");
    expect(res).toEqual({ ok: false, error: "Tin đăng không còn tồn tại." });
  });

  // Scenario: Moderation UI: A forged action by a non-admin is refused
  it("maps permission denied", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-9",
      name: "buyer",
      scopes: [],
    });
    vi.mocked(moderateListing).mockResolvedValue({ status: "forbidden" });
    const res = await moderateListingAction("l1", "ban", "other", "");
    expect(res).toEqual({ ok: false, error: PERM });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  // Scenario: Moderation UI: Expired session is sent to login
  it("redirects to login when the gateway says unauthenticated", async () => {
    vi.mocked(moderateListing).mockResolvedValue({ status: "unauthenticated" });
    await expect(
      moderateListingAction("l1", "approve", "", ""),
    ).rejects.toThrow("NEXT_REDIRECT");
    expect(redirectGuestToLogin).toHaveBeenCalledWith("/admin/listings");
  });

  // Scenario: Moderation UI: Audit unavailable leaves the row
  it("never shows err.message and never logs the note", async () => {
    const spies = [
      vi.spyOn(console, "log").mockImplementation(() => {}),
      vi.spyOn(console, "error").mockImplementation(() => {}),
      vi.spyOn(console, "warn").mockImplementation(() => {}),
      vi.spyOn(console, "info").mockImplementation(() => {}),
    ];
    vi.mocked(moderateListing).mockRejectedValue(new Error("pg-1:5432 down"));
    const res = await moderateListingAction(
      "l1",
      "reject",
      "other",
      "BÍ-MẬT-NOTE",
    );
    expect(res.ok).toBe(false);
    expect(JSON.stringify(res)).not.toContain("pg-1");
    for (const s of spies) {
      expect(JSON.stringify(s.mock.calls)).not.toContain("BÍ-MẬT-NOTE");
    }
  });
});
