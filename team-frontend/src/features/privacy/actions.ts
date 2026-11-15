"use server";

import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";

import { type ActionResult, fail, ok } from "@/lib/action-result";
import {
  type ConsentPurposeName,
  cancelDeletion,
  exportMyData,
  recordConsents,
  requestDeletion,
} from "@/lib/gateway/privacy";

const PAGE = "/account/privacy";
const CONSENT_VERSION = "1";
const SESSION_EXPIRED = "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.";

function codeOf(e: unknown): Code | undefined {
  return e instanceof ConnectError ? e.code : undefined;
}

export async function saveConsentAction(
  changes: { purpose: ConsentPurposeName; granted: boolean }[],
): Promise<ActionResult> {
  try {
    await recordConsents(
      changes.map((c) => ({
        purpose: c.purpose,
        granted: c.granted,
        version: CONSENT_VERSION,
        source: "account",
      })),
    );
  } catch (e) {
    const code = codeOf(e);
    if (code === Code.Unauthenticated) return fail(SESSION_EXPIRED);
    if (code === Code.InvalidArgument) return fail("Lựa chọn không hợp lệ.");
    return fail("Chưa lưu được lựa chọn. Vui lòng thử lại.");
  }
  revalidatePath(PAGE);
  return ok();
}

export async function exportDataAction(): Promise<
  ActionResult<{ filename: string; content: string }>
> {
  try {
    const res = await exportMyData();
    return ok({
      filename: `agora-du-lieu-${res.generatedAt.slice(0, 10)}.json`,
      content: res.content,
    });
  } catch (e) {
    const code = codeOf(e);
    if (code === Code.Unauthenticated || code === Code.PermissionDenied) {
      return fail(SESSION_EXPIRED);
    }
    return fail("Chưa tải được dữ liệu. Vui lòng thử lại sau.");
  }
}

export async function requestDeletionAction(
  password: string,
): Promise<ActionResult<{ eraseAfter: string }>> {
  let eraseAfter: string;
  try {
    ({ eraseAfter } = await requestDeletion(password));
  } catch (e) {
    const code = codeOf(e);
    if (code === Code.Unauthenticated) return fail(SESSION_EXPIRED);
    if (code === Code.FailedPrecondition && e instanceof ConnectError) {
      return fail(`Chưa thể xóa tài khoản: ${e.rawMessage}`);
    }
    if (code === Code.Unavailable) {
      return fail(
        "Chưa kiểm tra được đơn hàng và số dư của bạn. Vui lòng thử lại sau.",
      );
    }
    if (code === Code.PermissionDenied) return fail("Mật khẩu không đúng.");
    if (code === Code.InvalidArgument) return fail("Vui lòng nhập mật khẩu.");
    return fail("Chưa gửi được yêu cầu xóa tài khoản. Vui lòng thử lại sau.");
  }
  revalidatePath(PAGE);
  return ok({ eraseAfter });
}

export async function cancelDeletionAction(): Promise<ActionResult> {
  try {
    await cancelDeletion();
  } catch (e) {
    const code = codeOf(e);
    if (code === Code.Unauthenticated) return fail(SESSION_EXPIRED);
    if (code === Code.FailedPrecondition) {
      revalidatePath(PAGE);
      return fail("Không có yêu cầu xóa tài khoản đang chờ.");
    }
    return fail("Chưa hủy được yêu cầu xóa tài khoản. Vui lòng thử lại sau.");
  }
  revalidatePath(PAGE);
  return ok();
}
