"use server";

import { revalidatePath } from "next/cache";

import { createReferralCode, redeemReferral } from "@/lib/gateway/referral";

export async function ensureReferralCodeAction(): Promise<{
  ok: boolean;
  code?: string;
  message?: string;
}> {
  try {
    const code = await createReferralCode();
    revalidatePath("/account/referral");
    return { ok: true, code };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Tạo mã giới thiệu thất bại.",
    };
  }
}

export async function redeemReferralAction(
  code: string,
): Promise<{ ok: boolean; message?: string }> {
  if (!code.trim()) {
    return { ok: false, message: "Vui lòng nhập mã giới thiệu." };
  }
  try {
    await redeemReferral(code);
    revalidatePath("/account/referral");
    return { ok: true };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Nhập mã giới thiệu thất bại.",
    };
  }
}
