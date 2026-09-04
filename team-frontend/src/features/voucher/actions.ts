"use server";

import { revalidatePath } from "next/cache";

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

export interface CreateVoucherResult {
  ok: boolean;
  message: string;
  voucher?: ViewVoucher;
}

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
    return { ok: true, message: "Tạo voucher thành công!", voucher };
  } catch (err) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Tạo voucher thất bại.",
    };
  }
}
