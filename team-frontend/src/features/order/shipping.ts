/** Orders at or above this subtotal ship free. */
export const FREE_SHIPPING_THRESHOLD = 500000;

export interface ShippingFee {
  fee: number;
  isFree: boolean;
}

/**
 * Display estimate of the shipping fee (the authoritative amount is set by
 * team-order): free from 500000, 20000 for Ho Chi Minh and Ha Noi, else 35000.
 */
export function computeShippingFee(
  city: string,
  subtotal: number,
): ShippingFee {
  if (subtotal >= FREE_SHIPPING_THRESHOLD) {
    return { fee: 0, isFree: true };
  }
  const cityUpper = city.toUpperCase();
  if (
    cityUpper.includes("HỒ CHÍ MINH") ||
    cityUpper.includes("HCM") ||
    cityUpper.includes("HÀ NỘI") ||
    cityUpper.includes("HN")
  ) {
    return { fee: 20000, isFree: false };
  }
  return { fee: 35000, isFree: false };
}
