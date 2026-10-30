import { formatPrice } from "./products";

export const DEFAULT_CURRENCY = "VND";

export function formatMinor(
  minor: bigint | number | string,
  currency?: string,
) {
  return formatPrice(minor, currency || DEFAULT_CURRENCY);
}
