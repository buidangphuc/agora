"use server";

import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";

import { errorMessage } from "@/features/account/action-error";
import { type ActionResult, fail, ok } from "@/lib/action-result";
import {
  createReferralCode,
  getMyReferral,
  redeemReferral,
} from "@/lib/gateway/referral";
import { createShareLink } from "@/lib/gateway/sharing";

const INVITE_LINK_ERROR = "Không tạo được link mời. Vui lòng thử lại.";
const REDEEM_REFUSED =
  "Không thể nhập mã: tài khoản đã quá 7 ngày, đã có đơn hoàn thành, đã nhập mã hoặc mã là của chính bạn.";

export async function createReferralInviteLinkAction(): Promise<
  ActionResult<{ shortCode: string }>
> {
  try {
    const { code } = await getMyReferral();
    if (!code) return fail(INVITE_LINK_ERROR);
    const shortCode = await createShareLink("referral", code, {
      utm_source: "referral",
      utm_medium: "invite",
    });
    return ok({ shortCode });
  } catch {
    return fail(INVITE_LINK_ERROR);
  }
}

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
    if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
      return fail(err.rawMessage || REDEEM_REFUSED);
    }
    return fail(errorMessage(err, "Nhập mã giới thiệu thất bại."));
  }
}
