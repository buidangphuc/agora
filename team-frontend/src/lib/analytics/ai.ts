import { trackEcommerce } from "@/lib/analytics";

export type AiSurface =
  | "assistant"
  | "chat_copilot"
  | "magic_listing"
  | "review_summary";
export type AiAction = "shown" | "accepted" | "edited";

/**
 * Reports one AI interaction. Only the surface, the action and (optionally)
 * the listing id are sent; never free text. A tracker failure never breaks
 * the AI action.
 */
export function trackAiInteraction(
  surface: AiSurface,
  action: AiAction,
  listingId?: string,
): void {
  try {
    trackEcommerce("ai_interaction", {
      properties: { surface, action },
      items: listingId ? [{ itemId: listingId }] : undefined,
    });
  } catch {
    // analytics must never break the AI feature
  }
}
