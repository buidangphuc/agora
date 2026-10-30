"use server";

import { revalidatePath } from "next/cache";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { type ActionResult, fail, ok } from "@/lib/action-result";
import {
  type DisputeRulingName,
  type DisputeStatusName,
  type ViewDispute,
  resolveDispute,
} from "@/lib/gateway/disputes";
import { getPrincipal } from "@/lib/gateway/session";

const TRANSITION_ERROR = "Không thể chuyển khiếu nại sang trạng thái này.";
const AUDIT_ERROR =
  "Hệ thống kiểm toán tạm thời không khả dụng. Chưa có thay đổi nào được lưu.";
const TIMEOUT_ERROR =
  "Chưa rõ thao tác đã được lưu hay chưa. Vui lòng tải lại trang để xem trạng thái hiện tại.";
const ADMIN_ONLY_ERROR = "Bạn không có quyền thực hiện thao tác này.";

export async function resolveDisputeAction(input: {
  disputeId: string;
  status: Exclude<DisputeStatusName, "OPEN">;
  resolution: string;
  ruling?: Exclude<DisputeRulingName, "">;
}): Promise<ActionResult<ViewDispute>> {
  const principal = getPrincipal();
  if (!principal) return redirectGuestToLogin("/admin/cases");
  if (!principal.scopes.includes("admin")) return fail(ADMIN_ONLY_ERROR);

  let res: Awaited<ReturnType<typeof resolveDispute>>;
  try {
    res = await resolveDispute(input);
  } catch {
    return fail(AUDIT_ERROR);
  }
  switch (res.status) {
    case "ok":
      revalidatePath("/admin/cases");
      return ok(res.dispute);
    case "timeout":
      revalidatePath("/admin/cases");
      return fail(TIMEOUT_ERROR);
    case "unavailable":
      return fail(AUDIT_ERROR);
    case "unauthenticated":
      return redirectGuestToLogin("/admin/cases");
    default:
      return fail(TRANSITION_ERROR);
  }
}
