"use server";

import { type VoucherPreview, previewVoucher } from "@/lib/gateway/promotion";

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
