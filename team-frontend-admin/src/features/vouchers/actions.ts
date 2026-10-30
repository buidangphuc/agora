"use server";

import { revalidatePath } from "next/cache";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { getPrincipal } from "@/lib/gateway/session";
import {
  type VoucherView,
  createPlatformVoucher,
} from "@/lib/gateway/vouchers";

import {
  type VoucherFieldErrors,
  type VoucherFormValues,
  validateVoucherForm,
} from "./voucherForm";
import { VOUCHER_MESSAGES } from "./voucherMessages";

export type CreateVoucherActionResult =
  | { ok: true; voucher: VoucherView }
  | { ok: false; error: string; fieldErrors?: VoucherFieldErrors };

const LIST_PATH = "/admin/vouchers";
const NEW_PATH = "/admin/vouchers/new";

const fail = (
  error: string,
  fieldErrors?: VoucherFieldErrors,
): CreateVoucherActionResult =>
  fieldErrors ? { ok: false, error, fieldErrors } : { ok: false, error };

export async function createPlatformVoucherAction(
  values: VoucherFormValues,
): Promise<CreateVoucherActionResult> {
  const principal = getPrincipal();
  if (!principal) return redirectGuestToLogin(NEW_PATH);
  if (!principal.scopes.includes("admin")) {
    return fail(VOUCHER_MESSAGES.forbidden);
  }

  const checked = validateVoucherForm(values, new Date());
  if (!checked.ok) return fail(VOUCHER_MESSAGES.fields, checked.errors);

  const res = await createPlatformVoucher(checked.input);
  switch (res.status) {
    case "ok":
      revalidatePath(LIST_PATH);
      return { ok: true, voucher: res.voucher };
    case "unauthenticated":
      return redirectGuestToLogin(NEW_PATH);
    case "forbidden":
      return fail(VOUCHER_MESSAGES.forbidden);
    case "invalid":
      return fail(VOUCHER_MESSAGES.invalid);
    case "unavailable":
      return fail(VOUCHER_MESSAGES.auditDown);
    case "timeout":
      return fail(VOUCHER_MESSAGES.timeout);
    default:
      return fail(VOUCHER_MESSAGES.failed);
  }
}
