"use server";

import { revalidatePath } from "next/cache";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import {
  type ModerationDecisionName,
  moderateListing,
} from "@/lib/gateway/moderation";
import { getPrincipal } from "@/lib/gateway/session";

export type ModerateListingActionResult =
  | { ok: true; message: string }
  | { ok: false; error: string };

const PATH = "/admin/listings";
const MAX_NOTE = 500;

const fail = (error: string): ModerateListingActionResult => ({
  ok: false,
  error,
});

const DONE: Record<ModerationDecisionName, string> = {
  approve: "Đã duyệt tin đăng.",
  reject: "Đã từ chối tin đăng.",
  ban: "Đã khóa tin đăng.",
};

const PERM = "Bạn không có quyền thực hiện thao tác này.";

export async function moderateListingAction(
  listingId: string,
  decision: ModerationDecisionName,
  reasonCode: string,
  note: string,
): Promise<ModerateListingActionResult> {
  const principal = getPrincipal();
  if (!principal) redirectGuestToLogin(PATH);
  if (!principal.scopes.includes("admin")) return fail(PERM);

  const reason = reasonCode.trim();
  const trimmedNote = note.trim();
  if (!listingId || !(decision in DONE)) {
    return fail("Dữ liệu không hợp lệ; tin đăng không thay đổi.");
  }
  if (decision !== "approve" && !reason) return fail("Vui lòng chọn lý do.");
  if (trimmedNote.length > MAX_NOTE) return fail("Ghi chú tối đa 500 ký tự.");

  let res: Awaited<ReturnType<typeof moderateListing>>;
  try {
    res = await moderateListing(listingId, decision, reason, trimmedNote);
  } catch {
    res = { status: "unavailable" };
  }
  switch (res.status) {
    case "ok":
      revalidatePath(PATH);
      revalidatePath("/admin/cases");
      return { ok: true, message: DONE[decision] };
    case "unauthenticated":
      return redirectGuestToLogin(PATH);
    case "forbidden":
      return fail(PERM);
    case "not_found":
      return fail("Tin đăng không còn tồn tại.");
    case "failed_precondition":
      return fail(
        "Trạng thái hiện tại của tin đăng không cho phép thao tác này.",
      );
    case "invalid":
      return fail("Dữ liệu không hợp lệ; tin đăng không thay đổi.");
    default:
      return fail(
        "Chưa ghi được nhật ký kiểm toán nên thao tác chưa được thực hiện. Vui lòng thử lại sau.",
      );
  }
}
