import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { createReferralCode, redeemReferral } from "@/lib/gateway/referral";

import { ensureReferralCodeAction, redeemReferralAction } from "./actions";

vi.mock("@/lib/gateway/referral", () => ({
  createReferralCode: vi.fn(),
  redeemReferral: vi.fn(),
}));

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
