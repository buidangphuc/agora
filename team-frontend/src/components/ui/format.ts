export { formatPrice } from "@fe-platform/ui";

/** Format sold count like Shopee (e.g. 1.2k đã bán, 150 đã bán) */
export function formatSoldCount(count: number): string {
  if (!count || count <= 0) return "Đã bán 0";
  if (count >= 1000) {
    return `Đã bán ${(count / 1000).toFixed(1)}k`;
  }
  return `Đã bán ${count}`;
}
