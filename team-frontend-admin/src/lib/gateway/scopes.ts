import type { SessionPrincipal } from "./session";

export const PROMOTION_ADMIN_SCOPES = ["admin", "promotion.admin"] as const;

export function isPromotionAdmin(principal: SessionPrincipal): boolean {
  return PROMOTION_ADMIN_SCOPES.every((s) => principal.scopes.includes(s));
}
