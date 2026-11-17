"use server";

import { revalidatePath } from "next/cache";

import { Code, ConnectError } from "@connectrpc/connect";

import { type ActionResult, fail, ok } from "@/lib/action-result";
import { createKycUploadUrl, submitKyc } from "@/lib/gateway/verification";

import { KYC_ACCEPTED_TYPES, MAX_KYC_FILE_BYTES } from "./status";

const UPLOAD_FAILED = "Tải tệp thất bại, vui lòng thử lại";

export async function createKycUploadUrlAction(
  contentType: string,
  sizeBytes: number,
): Promise<ActionResult<{ uploadUrl: string; docRef: string }>> {
  if (
    !KYC_ACCEPTED_TYPES.includes(contentType) ||
    !Number.isInteger(sizeBytes) ||
    sizeBytes <= 0 ||
    sizeBytes > MAX_KYC_FILE_BYTES
  ) {
    return fail("Tệp không hợp lệ");
  }
  try {
    const res = await createKycUploadUrl(contentType, sizeBytes);
    return ok({ uploadUrl: res.uploadUrl, docRef: res.docRef });
  } catch {
    return fail(UPLOAD_FAILED);
  }
}

export async function submitKycAction(
  docType: string,
  docRefs: string[],
): Promise<ActionResult> {
  if (docRefs.length === 0) return fail("Vui lòng chọn tệp để gửi");
  const [first, ...rest] = docRefs;
  try {
    await submitKyc(docType, first, rest);
  } catch (err: unknown) {
    if (
      err instanceof ConnectError &&
      (err.code === Code.InvalidArgument || err.code === Code.PermissionDenied)
    ) {
      return fail("Hồ sơ không hợp lệ, vui lòng tải lại tệp");
    }
    return fail("Gửi hồ sơ thất bại, vui lòng thử lại");
  }
  revalidatePath("/seller/verification");
  return ok();
}
