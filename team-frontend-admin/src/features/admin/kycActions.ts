"use server";

import { revalidatePath } from "next/cache";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { getKycDocumentUrl, reviewKyc } from "@/lib/gateway/kyc";
import { getPrincipal } from "@/lib/gateway/session";

import { isKycReasonCode } from "./kycReasons";

export type ViewKycDocumentResult =
  | { ok: true; url: string; expiresAt: string }
  | { ok: false; error: string };

export type ReviewKycActionResult =
  | { ok: true; message: string }
  | { ok: false; error: string };

const PATH = "/admin/cases";
const PERM = "Bạn không có quyền thực hiện thao tác này.";
const BAD_INPUT = "Dữ liệu không hợp lệ; hồ sơ không thay đổi.";

function field(fd: FormData, key: string): string {
  const v = fd.get(key);
  return typeof v === "string" ? v : "";
}

function requireAdmin(): boolean {
  const principal = getPrincipal();
  if (!principal) redirectGuestToLogin(PATH);
  return principal.scopes.includes("admin");
}

export async function viewKycDocumentAction(
  formData: FormData,
): Promise<ViewKycDocumentResult> {
  if (!requireAdmin()) return { ok: false, error: PERM };

  const id = field(formData, "submissionId").trim();
  const rawIndex = field(formData, "index");
  if (!id || !/^\d+$/.test(rawIndex)) {
    return { ok: false, error: "Không tìm thấy tài liệu này." };
  }

  let res: Awaited<ReturnType<typeof getKycDocumentUrl>>;
  try {
    res = await getKycDocumentUrl(id, Number(rawIndex));
  } catch {
    res = { status: "unavailable" };
  }
  switch (res.status) {
    case "ok":
      return { ok: true, url: res.url, expiresAt: res.expiresAt };
    case "unauthenticated":
      return redirectGuestToLogin(PATH);
    case "forbidden":
      return { ok: false, error: PERM };
    case "not_found":
    case "invalid":
      return { ok: false, error: "Không tìm thấy tài liệu này." };
    default:
      return {
        ok: false,
        error:
          "Không ghi được nhật ký truy cập nên chưa mở được tài liệu. Vui lòng thử lại sau.",
      };
  }
}

export async function reviewKycAction(
  formData: FormData,
): Promise<ReviewKycActionResult> {
  if (!requireAdmin()) return { ok: false, error: PERM };

  const id = field(formData, "submissionId").trim();
  const decision = field(formData, "decision");
  const reason = field(formData, "reason").trim();
  if (!id || (decision !== "approve" && decision !== "reject")) {
    return { ok: false, error: BAD_INPUT };
  }
  if (decision === "reject" && !isKycReasonCode(reason)) {
    return { ok: false, error: BAD_INPUT };
  }

  let res: Awaited<ReturnType<typeof reviewKyc>>;
  try {
    res = await reviewKyc(id, decision, decision === "approve" ? "" : reason);
  } catch {
    res = { status: "unavailable" };
  }
  switch (res.status) {
    case "ok":
      revalidatePath(PATH);
      return {
        ok: true,
        message:
          decision === "approve"
            ? "Đã duyệt hồ sơ xác minh."
            : "Đã từ chối hồ sơ xác minh.",
      };
    case "unauthenticated":
      return redirectGuestToLogin(PATH);
    case "forbidden":
      return {
        ok: false,
        error:
          "Bạn không thể xử lý hồ sơ của chính mình hoặc không có quyền thực hiện thao tác này.",
      };
    case "failed_precondition":
      return {
        ok: false,
        error: "Hồ sơ đã được xử lý hoặc không còn ở trạng thái chờ.",
      };
    case "not_found":
    case "invalid":
      return { ok: false, error: BAD_INPUT };
    default:
      return {
        ok: false,
        error: "Không xử lý được hồ sơ. Vui lòng thử lại sau.",
      };
  }
}
