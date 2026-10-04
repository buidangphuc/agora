import type { ViewAddress } from "@/lib/gateway/addresses";

/** One-line address; a plain module so server and client components can share it. */
export function formatAddress(a: ViewAddress): string {
  return [a.street, a.ward, a.district, a.city].filter(Boolean).join(", ");
}
