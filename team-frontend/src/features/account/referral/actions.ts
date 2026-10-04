"use server";

import { revalidatePath } from "next/cache";

import { errorMessage } from "@/features/account/action-error";
import { type ActionResult, fail, ok } from "@/lib/action-result";
import { createReferralCode, redeemReferral } from "@/lib/gateway/referral";

export async function ensureReferralCodeAction(): Promise<
  ActionResult<{ code: string }>
> {
  try {
    const code = await createReferralCode();
    revalidatePath("/account/referral");
    return ok({ code });
  } catch (err: unknown) {
    return fail(errorMessage(err, "Tạo mã giới thiệu thất bại."));
  }
}

export async function redeemReferralAction(
  code: string,
): Promise<ActionResult> {
  if (!code.trim()) {
    return fail("Vui lòng nhập mã giới thiệu.");
  }
  try {
    await redeemReferral(code);
    revalidatePath("/account/referral");
    return ok();
  } catch (err: unknown) {
    return fail(errorMessage(err, "Nhập mã giới thiệu thất bại."));
  }
}
