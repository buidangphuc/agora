"use server";

import { revalidatePath } from "next/cache";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { type ActionResult, fail, ok } from "@/lib/action-result";
import {
  type DisputeRulingName,
  type DisputeStatusName,
  type ViewDispute,
  createDispute,
  resolveDispute,
} from "@/lib/gateway/disputes";
import { getPrincipal } from "@/lib/gateway/session";

const CREATE_ERRORS = {
  failed_precondition: "Yêu cầu trả hàng này chưa thể khiếu nại.",
  already_exists: "Yêu cầu trả hàng này đã có khiếu nại đang xử lý.",
  permission_denied: "Bạn không có quyền khiếu nại đơn hàng này.",
  not_found: "Không tìm thấy đơn hàng hoặc yêu cầu trả hàng này.",
  invalid_argument:
    "Không gửi được khiếu nại. Vui lòng tải lại trang và thử lại.",
  unavailable: "Không gửi được khiếu nại. Vui lòng thử lại sau.",
} as const;

const TRANSITION_ERROR = "Không thể chuyển khiếu nại sang trạng thái này.";
const AUDIT_ERROR =
  "Hệ thống kiểm toán tạm thời không khả dụng. Chưa có thay đổi nào được lưu.";
const TIMEOUT_ERROR =
  "Chưa rõ thao tác đã được lưu hay chưa. Vui lòng tải lại trang để xem trạng thái hiện tại.";
const ADMIN_ONLY_ERROR = "Bạn không có quyền thực hiện thao tác này.";

export async function createDisputeAction(input: {
  orderId: string;
  defendantId: string;
  returnId: string;
  reason: string;
}): Promise<ActionResult<ViewDispute>> {
  const orderPath = `/account/orders/${input.orderId}`;
  let res: Awaited<ReturnType<typeof createDispute>>;
  try {
    res = await createDispute(input);
  } catch {
    return fail(CREATE_ERRORS.unavailable);
  }
  if (res.status === "ok") {
    revalidatePath(orderPath);
    return ok(res.dispute);
  }
  if (res.status === "unauthenticated") return redirectGuestToLogin(orderPath);
  if (res.status === "already_exists") revalidatePath(orderPath);
  return fail(CREATE_ERRORS[res.status]);
}

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
