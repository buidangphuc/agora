"use server";

import { revalidatePath } from "next/cache";

import { errorMessage } from "@/features/account/action-error";
import { type ActionResult, fail, ok } from "@/lib/action-result";
import { submitKyc } from "@/lib/gateway/verification";

export async function submitKycAction(
  docType: string,
  docRef: string,
): Promise<ActionResult> {
  if (!docType.trim() || !docRef.trim()) {
    return fail("Vui lòng chọn loại giấy tờ và nhập mã tham chiếu.");
  }
  try {
    await submitKyc(docType, docRef);
    revalidatePath("/account/verification");
    return ok();
  } catch (err: unknown) {
    return fail(errorMessage(err, "Gửi hồ sơ xác minh thất bại."));
  }
}
