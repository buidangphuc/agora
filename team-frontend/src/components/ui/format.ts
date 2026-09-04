/** Format a price for Vietnamese Shopee locale with leading ₫ symbol (e.g. ₫29.990.000). */
export function formatPrice(price: number, _currency = "VND"): string {
  if (price === undefined || price === null) return "₫0";
  try {
    const formatted = new Intl.NumberFormat("vi-VN").format(price);
    return `₫${formatted}`;
  } catch {
    return `₫${price.toLocaleString("vi-VN")}`;
  }
}

/** Format sold count like Shopee (e.g. 1.2k đã bán, 150 đã bán) */
export function formatSoldCount(count: number): string {
  if (!count || count <= 0) return "Đã bán 0";
  if (count >= 1000) {
    return `Đã bán ${(count / 1000).toFixed(1)}k`;
  }
  return `Đã bán ${count}`;
}
