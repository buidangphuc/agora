import { defineSession } from "@fe-platform/session";

import { sellerAppUrl } from "@/lib/apps";

const DEFAULT_BUYER_APP_URL = "http://localhost:3000";

export const buyerHandoffSession = defineSession("buyer");

export function buyerOrigin(): string {
  return (
    process.env.NEXT_PUBLIC_BUYER_APP_URL || DEFAULT_BUYER_APP_URL
  ).replace(/\/+$/, "");
}

export function sellerOrigin(): string {
  return sellerAppUrl("/").replace(/\/+$/, "");
}
