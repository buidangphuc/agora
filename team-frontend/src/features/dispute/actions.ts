"use server";

import { revalidatePath } from "next/cache";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { type ActionResult, fail, ok } from "@/lib/action-result";
import { type ViewDispute, createDispute } from "@/lib/gateway/disputes";

const CREATE_ERRORS = {
  failed_precondition: "Yêu cầu trả hàng này chưa thể khiếu nại.",
  already_exists: "Yêu cầu trả hàng này đã có khiếu nại đang xử lý.",
  permission_denied: "Bạn không có quyền khiếu nại đơn hàng này.",
  not_found: "Không tìm thấy đơn hàng hoặc yêu cầu trả hàng này.",
  invalid_argument:
    "Không gửi được khiếu nại. Vui lòng tải lại trang và thử lại.",
  unavailable: "Không gửi được khiếu nại. Vui lòng thử lại sau.",
} as const;

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
