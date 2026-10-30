/** Formats a price in Vietnamese dong with a leading ₫ symbol, for example ₫29.990.000. */
export function formatPrice(price: number, _currency = "VND"): string {
  if (price === undefined || price === null) return "₫0";
  try {
    return `₫${new Intl.NumberFormat("vi-VN").format(price)}`;
  } catch {
    return `₫${price.toLocaleString("vi-VN")}`;
  }
}
