"use server";

import { revalidatePath } from "next/cache";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { getPrincipal } from "@/lib/gateway/session";
import {
  type VoucherView,
  createPlatformVoucher,
  deactivateVoucher,
  updateVoucher,
} from "@/lib/gateway/vouchers";

import {
  type VoucherEditErrors,
  type VoucherEditValues,
  type VoucherFieldErrors,
  type VoucherFormValues,
  validateEditForm,
  validateVoucherForm,
} from "./voucherForm";
import { VOUCHER_MESSAGES } from "./voucherMessages";

export type CreateVoucherActionResult =
  | { ok: true; voucher: VoucherView }
  | { ok: false; error: string; fieldErrors?: VoucherFieldErrors };

export type ChangeVoucherActionResult =
  | { ok: true; voucher: VoucherView }
  | { ok: false; error: string; fieldErrors?: VoucherEditErrors };

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
    case "audit_unavailable":
      return fail(VOUCHER_MESSAGES.auditDown);
    case "promotion_unavailable":
      return fail(VOUCHER_MESSAGES.promotionDown);
    case "duplicate":
      return fail(VOUCHER_MESSAGES.fields, {
        code: VOUCHER_MESSAGES.duplicate,
      });
    case "timeout":
      return fail(VOUCHER_MESSAGES.timeout);
    default:
      return fail(VOUCHER_MESSAGES.failed);
  }
}

function changeFailure(status: string): ChangeVoucherActionResult {
  switch (status) {
    case "forbidden":
      return { ok: false, error: VOUCHER_MESSAGES.forbidden };
    case "invalid":
      return { ok: false, error: VOUCHER_MESSAGES.invalidChange };
    case "audit_unavailable":
      return { ok: false, error: VOUCHER_MESSAGES.auditDownChange };
    case "promotion_unavailable":
      return { ok: false, error: VOUCHER_MESSAGES.promotionDownChange };
    case "timeout":
      return { ok: false, error: VOUCHER_MESSAGES.timeout };
    default:
      return { ok: false, error: VOUCHER_MESSAGES.failedChange };
  }
}

export async function deactivateVoucherAction(
  voucherId: string,
): Promise<ChangeVoucherActionResult> {
  const principal = getPrincipal();
  if (!principal) return redirectGuestToLogin(LIST_PATH);
  if (!principal.scopes.includes("admin")) {
    return { ok: false, error: VOUCHER_MESSAGES.forbidden };
  }

  const res = await deactivateVoucher(voucherId);
  if (res.status === "ok") {
    revalidatePath(LIST_PATH);
    return { ok: true, voucher: res.voucher };
  }
  if (res.status === "unauthenticated") return redirectGuestToLogin(LIST_PATH);
  return changeFailure(res.status);
}

export async function updateVoucherAction(
  current: VoucherView,
  values: VoucherEditValues,
): Promise<ChangeVoucherActionResult> {
  const principal = getPrincipal();
  if (!principal) return redirectGuestToLogin(LIST_PATH);
  if (!principal.scopes.includes("admin")) {
    return { ok: false, error: VOUCHER_MESSAGES.forbidden };
  }

  const checked = validateEditForm(values, current, new Date());
  if (!checked.ok) {
    return {
      ok: false,
      error: VOUCHER_MESSAGES.fields,
      fieldErrors: checked.errors,
    };
  }

  const res = await updateVoucher(current.id, checked.input);
  if (res.status === "ok") {
    revalidatePath(LIST_PATH);
    return { ok: true, voucher: res.voucher };
  }
  if (res.status === "unauthenticated") return redirectGuestToLogin(LIST_PATH);
  return changeFailure(res.status);
}
