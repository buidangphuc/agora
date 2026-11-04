import { buyerAppUrl } from "@/lib/apps";
import { sellerSession } from "@/lib/session-config";

const DEFAULT_SELLER_APP_URL = "http://localhost:3001";

export { sellerSession };

export function sellerOrigin(): string {
  return (
    process.env.NEXT_PUBLIC_SELLER_APP_URL || DEFAULT_SELLER_APP_URL
  ).replace(/\/+$/, "");
}

export function buyerOrigin(): string {
  return buyerAppUrl("/").replace(/\/+$/, "");
}

export function buyerLoginUrl(next: string): string {
  return `${buyerOrigin()}/login?next=${encodeURIComponent(next)}`;
}
