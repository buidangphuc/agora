import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { Code, ConnectError } from "@connectrpc/connect";

import {
  createReferralCode,
  getMyReferral,
  redeemReferral,
} from "@/lib/gateway/referral";
import { createShareLink } from "@/lib/gateway/sharing";

import {
  createReferralInviteLinkAction,
  ensureReferralCodeAction,
  redeemReferralAction,
} from "./actions";

vi.mock("@/lib/gateway/referral", () => ({
  createReferralCode: vi.fn(),
  getMyReferral: vi.fn(),
  redeemReferral: vi.fn(),
}));
vi.mock("@/lib/gateway/sharing", () => ({ createShareLink: vi.fn() }));

beforeEach(() => vi.clearAllMocks());

describe("ensureReferralCodeAction", () => {
  it("returns the new code and revalidates the referral page", async () => {
    vi.mocked(createReferralCode).mockResolvedValue("ABC123");
    await expect(ensureReferralCodeAction()).resolves.toEqual({
      ok: true,
      data: { code: "ABC123" },
    });
    expect(revalidatePath).toHaveBeenCalledWith("/account/referral");
  });

  it("resolves with { ok: false, error } on a gateway error", async () => {
    vi.mocked(createReferralCode).mockRejectedValue(new Error("down"));
    await expect(ensureReferralCodeAction()).resolves.toEqual({
      ok: false,
      error: "down",
    });
  });
});

describe("redeemReferralAction", () => {
  it("rejects an empty code without calling the gateway", async () => {
    const res = await redeemReferralAction("  ");
    expect(res.ok).toBe(false);
    expect(redeemReferral).not.toHaveBeenCalled();
  });

  it("redeems and revalidates", async () => {
    vi.mocked(redeemReferral).mockResolvedValue(undefined);
    await expect(redeemReferralAction("GOOD")).resolves.toEqual({ ok: true });
    expect(revalidatePath).toHaveBeenCalledWith("/account/referral");
  });

  it("resolves with the server message when the code is rejected", async () => {
    vi.mocked(redeemReferral).mockRejectedValue(new Error("Mã không hợp lệ"));
    await expect(redeemReferralAction("BAD")).resolves.toEqual({
      ok: false,
      error: "Mã không hợp lệ",
    });
  });
});

describe("createReferralInviteLinkAction", () => {
  const NO_CODE_FALLBACK = "Không tạo được link mời. Vui lòng thử lại.";

  it("S1 creates the share link for the caller's own code with referral utm", async () => {
    vi.mocked(getMyReferral).mockResolvedValue({
      code: "ABC123",
      invitedCount: 0,
      rewardsTotal: 0,
      pendingXu: 0,
      releasedXu: 0,
    });
    vi.mocked(createShareLink).mockResolvedValue("sh0rt");
    await expect(createReferralInviteLinkAction()).resolves.toEqual({
      ok: true,
      data: { shortCode: "sh0rt" },
    });
    expect(createShareLink).toHaveBeenCalledTimes(1);
    expect(createShareLink).toHaveBeenCalledWith("referral", "ABC123", {
      utm_source: "referral",
      utm_medium: "invite",
    });
  });

  it("S3 resolves with the Vietnamese failure message when CreateShareLink fails", async () => {
    vi.mocked(getMyReferral).mockResolvedValue({
      code: "ABC123",
      invitedCount: 0,
      rewardsTotal: 0,
      pendingXu: 0,
      releasedXu: 0,
    });
    vi.mocked(createShareLink).mockRejectedValue(new Error("boom"));
    await expect(createReferralInviteLinkAction()).resolves.toEqual({
      ok: false,
      error: NO_CODE_FALLBACK,
    });
  });

  it("S2 refuses without calling CreateShareLink when the caller has no code", async () => {
    vi.mocked(getMyReferral).mockResolvedValue({
      code: "",
      invitedCount: 0,
      rewardsTotal: 0,
      pendingXu: 0,
      releasedXu: 0,
    });
    const res = await createReferralInviteLinkAction();
    expect(res.ok).toBe(false);
    expect(createShareLink).not.toHaveBeenCalled();
  });
});

describe("redeemReferralAction refusal and success", () => {
  const FALLBACK =
    "Không thể nhập mã: tài khoản đã quá 7 ngày, đã có đơn hoàn thành, đã nhập mã hoặc mã là của chính bạn.";

  it("S23 surfaces the server's Vietnamese message on FAILED_PRECONDITION", async () => {
    vi.mocked(redeemReferral).mockRejectedValue(
      new ConnectError("Tài khoản đã quá 7 ngày.", Code.FailedPrecondition),
    );
    await expect(redeemReferralAction("ABC123")).resolves.toEqual({
      ok: false,
      error: "Tài khoản đã quá 7 ngày.",
    });
  });

  it("S23 uses the fallback message when FAILED_PRECONDITION carries no message", async () => {
    vi.mocked(redeemReferral).mockRejectedValue(
      new ConnectError("", Code.FailedPrecondition),
    );
    await expect(redeemReferralAction("ABC123")).resolves.toEqual({
      ok: false,
      error: FALLBACK,
    });
  });

  it("S23 does not revalidate when the redeem is refused", async () => {
    vi.mocked(redeemReferral).mockRejectedValue(
      new ConnectError("", Code.FailedPrecondition),
    );
    await redeemReferralAction("ABC123");
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("S24 an accepted redeem resolves ok and revalidates the page", async () => {
    vi.mocked(redeemReferral).mockResolvedValue(undefined);
    await expect(redeemReferralAction("ABC123")).resolves.toEqual({ ok: true });
    expect(redeemReferral).toHaveBeenCalledWith("ABC123");
    expect(revalidatePath).toHaveBeenCalledWith("/account/referral");
  });

  it("S25 a guest refusal is returned as the gateway message", async () => {
    vi.mocked(redeemReferral).mockRejectedValue(
      new ConnectError("Vui lòng đăng nhập.", Code.Unauthenticated),
    );
    await expect(redeemReferralAction("ABC123")).resolves.toEqual({
      ok: false,
      error: "Vui lòng đăng nhập.",
    });
  });
});
