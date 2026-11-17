import { trackEcommerce } from "@/lib/analytics";

export type AiSurface =
  | "assistant"
  | "chat_copilot"
  | "magic_listing"
  | "review_summary";
export interface AiProvenance {
  requestId: string;
  modelVersion: string;
}
export type AiAction = "shown" | "accepted" | "edited" | "dismissed";

/**
 * Reports one AI interaction. Only the surface, the action and (optionally)
 * the listing id are sent; never free text. A tracker failure never breaks
 * the AI action.
 */
export function trackAiInteraction(
  surface: AiSurface,
  action: AiAction,
  listingId?: string,
  provenance?: AiProvenance,
): void {
  try {
    trackEcommerce("ai_interaction", {
      properties: { surface, action },
      items: listingId ? [{ itemId: listingId }] : undefined,
      ...(provenance?.requestId ? { impressionId: provenance.requestId } : {}),
      ...(provenance?.modelVersion
        ? { modelVersion: provenance.modelVersion }
        : {}),
    });
  } catch {
    // analytics must never break the AI feature
  }
}
