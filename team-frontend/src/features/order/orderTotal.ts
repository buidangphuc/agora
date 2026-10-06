/** Payable total: max(0, subtotal - discount + shipping). All amounts in VND. */
export function orderTotal(
  subtotal: number,
  discount: number,
  shippingFee: number,
): number {
  return Math.max(0, subtotal - discount + shippingFee);
}
