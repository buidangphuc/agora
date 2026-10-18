"use server";

import { revalidatePath } from "next/cache";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { resolveReport } from "@/lib/gateway/reports";

export type CloseReportActionResult =
  | { ok: true; message: string }
  | { ok: false; error: string };

const PATH = "/admin/cases";
const MAX_NOTE = 500;

const fail = (error: string): CloseReportActionResult => ({ ok: false, error });

export async function closeReportAction(
  reportId: string,
  outcome: "actioned" | "dismissed",
  note: string,
): Promise<CloseReportActionResult> {
  const trimmed = note.trim();
  if (!reportId || (outcome !== "actioned" && outcome !== "dismissed")) {
    return fail("Dữ liệu không hợp lệ; báo cáo không thay đổi.");
  }
  if (!trimmed) return fail("Vui lòng nhập ghi chú xử lý.");
  if (trimmed.length > MAX_NOTE) return fail("Ghi chú tối đa 500 ký tự.");

  let res: Awaited<ReturnType<typeof resolveReport>>;
  try {
    res = await resolveReport(reportId, outcome, trimmed);
  } catch {
    res = { status: "unavailable" };
  }
  switch (res.status) {
    case "ok":
      revalidatePath(PATH);
      return { ok: true, message: "Đã đóng báo cáo." };
    case "unauthenticated":
      return redirectGuestToLogin(PATH);
    case "forbidden":
      return fail("Bạn không có quyền thực hiện thao tác này.");
    case "not_found":
      return fail("Báo cáo không còn tồn tại.");
    case "failed_precondition":
      revalidatePath(PATH);
      return fail("Báo cáo này đã được xử lý.");
    case "invalid":
      return fail("Dữ liệu không hợp lệ; báo cáo không thay đổi.");
    default:
      return fail(
        "Chưa ghi được nhật ký kiểm toán nên thao tác chưa được thực hiện. Vui lòng thử lại sau.",
      );
  }
}
