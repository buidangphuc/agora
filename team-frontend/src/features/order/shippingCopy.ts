/** Orders at or above this subtotal ship free (display copy; team-order decides). */
export const FREE_SHIPPING_THRESHOLD = 500000;

/** Lowest and highest standard fee, shown when no city quote is available. */
export const SHIPPING_FEE_RANGE = { min: 20000, max: 35000 } as const;

/** Shipping as the checkout views show it; the amounts come from team-order. */
export interface ShippingFee {
  fee: number;
  isFree: boolean;
}

/** Shown when CreateOrder refuses because the cart prices moved. */
export const PRICE_CHANGED_MESSAGE =
  "Giá một số sản phẩm trong giỏ đã thay đổi. Giỏ hàng đã được cập nhật, vui lòng kiểm tra tổng tiền mới rồi đặt hàng lại.";
