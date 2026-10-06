"use server";

import { revalidatePath } from "next/cache";

import { type ActionResult, fail, ok } from "@/lib/action-result";
import {
  type CreateVoucherInput,
  type ViewVoucher,
  type VoucherPreview,
  createVoucher,
  previewVoucher,
} from "@/lib/gateway/promotion";

/**
 * Server Action: preview a voucher's discount at checkout through the gateway
 * (team-promotion ValidateAndReserve). Never throws to the client — an invalid
 * or unknown code comes back as { valid:false, reason }.
 */
export async function previewVoucherAction(
  code: string,
  cartSubtotal: number,
  sellerId?: string,
): Promise<VoucherPreview> {
  if (!code.trim()) {
    return {
      valid: false,
      reason: "Vui lòng nhập mã giảm giá.",
      discountAmount: 0,
      voucherId: "",
    };
  }
  return previewVoucher(code, cartSubtotal, sellerId);
}

/** ActionResult plus the legacy `message` field (kept so existing callers still work). */
export type CreateVoucherResult = ActionResult<ViewVoucher> & {
  message: string;
};

/**
 * Server Action: a seller/admin creates a voucher through the gateway
 * (team-promotion CreateVoucher). Scope authorization is enforced at the gateway.
 */
export async function createVoucherAction(
  input: CreateVoucherInput,
): Promise<CreateVoucherResult> {
  try {
    const voucher = await createVoucher(input);
    revalidatePath("/vouchers");
    return { ...ok(voucher), message: "Tạo voucher thành công!" };
  } catch (err) {
    const error = err instanceof Error ? err.message : "Tạo voucher thất bại.";
    return { ...fail(error), message: error };
  }
}
