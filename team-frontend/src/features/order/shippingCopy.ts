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

/** Shown when the address was edited after the buyer confirmed it. */
export const ADDRESS_CHANGED_MESSAGE =
  "Địa chỉ giao hàng đã thay đổi. Trang đã được cập nhật, vui lòng kiểm tra lại địa chỉ rồi đặt hàng.";

/** Shown when the chosen address no longer exists or none was chosen. */
export const ADDRESS_NOT_FOUND_MESSAGE =
  "Không tìm thấy địa chỉ giao hàng. Vui lòng chọn hoặc thêm địa chỉ để tiếp tục.";

/** Shown when a dependency (identity) is briefly unavailable. */
export const UNAVAILABLE_RETRY_MESSAGE =
  "Hệ thống đang bận, đơn hàng chưa được đặt. Vui lòng thử lại sau giây lát.";
