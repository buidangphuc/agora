"use server";

import { createReport } from "@/lib/gateway/reports";

export type ReportListingActionResult =
  | { ok: true; message: string }
  | { ok: false; error: string };

const MAX_NOTE = 500;

const fail = (error: string): ReportListingActionResult => ({
  ok: false,
  error,
});

const DOWN = "Chưa gửi được báo cáo. Vui lòng thử lại sau.";

export async function reportListingAction(
  listingId: string,
  reason: string,
  note: string,
): Promise<ReportListingActionResult> {
  const r = reason.trim();
  const n = note.trim();
  if (!r) return fail("Vui lòng chọn lý do.");
  if (n.length > MAX_NOTE) return fail("Ghi chú tối đa 500 ký tự.");

  let res: Awaited<ReturnType<typeof createReport>>;
  try {
    res = await createReport(listingId, r, n);
  } catch {
    res = { status: "unavailable" };
  }
  switch (res.status) {
    case "ok":
      return { ok: true, message: "Đã gửi báo cáo. Cảm ơn bạn." };
    case "failed_precondition":
      return fail("Bạn không thể báo cáo tin đăng của chính mình.");
    case "forbidden":
      return fail("Bạn không có quyền thực hiện thao tác này.");
    case "resource_exhausted":
      return fail(
        "Bạn đang có quá nhiều báo cáo chờ xử lý. Vui lòng thử lại sau khi các báo cáo được xử lý.",
      );
    case "unauthenticated":
      return fail("Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.");
    case "invalid":
      return fail("Dữ liệu không hợp lệ; báo cáo chưa được gửi.");
    default:
      return fail(DOWN);
  }
}
