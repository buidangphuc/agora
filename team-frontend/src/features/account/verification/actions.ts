"use server";

import { revalidatePath } from "next/cache";

import { submitKyc } from "@/lib/gateway/verification";

export async function submitKycAction(
  docType: string,
  docRef: string,
): Promise<{ ok: boolean; message?: string }> {
  if (!docType.trim() || !docRef.trim()) {
    return { ok: false, message: "Vui lòng chọn loại giấy tờ và nhập mã tham chiếu." };
  }
  try {
    await submitKyc(docType, docRef);
    revalidatePath("/account/verification");
    return { ok: true };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Gửi hồ sơ xác minh thất bại.",
    };
  }
}
