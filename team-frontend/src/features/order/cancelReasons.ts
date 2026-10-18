import { CancelReason } from "@/generated/platform/order/v1/order_pb.js";

export interface CancelReasonOption {
  value: string;
  label: string;
  code: CancelReason;
}

/** The only reasons a buyer may cancel with. */
export const CANCEL_REASONS: readonly CancelReasonOption[] = [
  {
    value: "changed_mind",
    label: "Tôi muốn thay đổi địa chỉ hoặc đổi ý",
    code: CancelReason.CHANGED_MIND,
  },
  {
    value: "wrong_item",
    label: "Shop tư vấn hoặc giao sai phân loại",
    code: CancelReason.WRONG_ITEM,
  },
  {
    value: "damaged",
    label: "Sản phẩm bị lỗi / hư hỏng",
    code: CancelReason.DAMAGED,
  },
  {
    value: "delivery_delay",
    label: "Thời gian giao hàng quá lâu",
    code: CancelReason.DELIVERY_DELAY,
  },
];

export function findCancelReason(
  value: string | undefined,
): CancelReasonOption | undefined {
  return CANCEL_REASONS.find((r) => r.value === value);
}
