"use server";

import { revalidatePath } from "next/cache";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { getPrincipal } from "@/lib/gateway/session";
import { setUserStatus, type UserStatusName } from "@/lib/gateway/users";

export type UserStatusActionResult =
  { ok: true; message: string } | { ok: false; error: string };

const PATH = "/admin/users";
const STATUSES: UserStatusName[] = [
  "active",
  "restricted",
  "suspended",
  "banned",
];
const MAX_NOTE = 500;

const fail = (error: string): UserStatusActionResult => ({ ok: false, error });

function field(fd: FormData, key: string): string {
  const v = fd.get(key);
  return typeof v === "string" ? v : "";
}

/** Parses a datetime-local value (YYYY-MM-DDTHH:mm) as UTC+7. */
function parseUntil(raw: string): Date | null {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(raw)) return null;
  const d = new Date(`${raw}:00+07:00`);
  return Number.isNaN(d.getTime()) ? null : d;
}

export async function setUserStatusAction(
  formData: FormData,
): Promise<UserStatusActionResult> {
  const principal = getPrincipal();
  if (!principal) redirectGuestToLogin(PATH);
  if (!principal.scopes.includes("admin")) {
    return fail("Bạn không có quyền thực hiện thao tác này.");
  }

  const userId = field(formData, "userId");
  const status = field(formData, "status") as UserStatusName;
  const reason = field(formData, "reason");
  const note = field(formData, "note");
  if (!userId || !STATUSES.includes(status)) {
    return fail("Dữ liệu không hợp lệ; trạng thái không thay đổi.");
  }
  if (!reason) return fail("Vui lòng chọn mã lý do.");
  if (note.length > MAX_NOTE) return fail("Ghi chú tối đa 500 ký tự.");

  let until: Date | undefined;
  if (status === "suspended") {
    const d = parseUntil(field(formData, "until"));
    if (!d || d.getTime() <= Date.now()) {
      return fail("Chọn thời điểm kết thúc tạm khóa trong tương lai.");
    }
    until = d;
  }

  const res = await setUserStatus({
    userId,
    status,
    reasonCode: reason,
    note,
    until,
  });
  switch (res.status) {
    case "ok":
      revalidatePath(PATH);
      return {
        ok: true,
        message: `Đã cập nhật trạng thái; đã đăng xuất ${res.sessionsRevoked} phiên.`,
      };
    case "unauthenticated":
      return redirectGuestToLogin(PATH);
    case "forbidden":
      return fail("Bạn không có quyền thực hiện thao tác này.");
    case "invalid":
      return fail("Dữ liệu không hợp lệ; trạng thái không thay đổi.");
    case "not_found":
      return fail("Không tìm thấy người dùng này; trạng thái không thay đổi.");
    case "failed_precondition":
      return fail(
        "Không thể thực hiện thao tác này; trạng thái không thay đổi.",
      );
    default:
      return fail(
        "Dịch vụ tạm thời không khả dụng; trạng thái không thay đổi.",
      );
  }
}
