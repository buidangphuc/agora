/**
 * Server-side AI data functions. They wrap the gateway AIService RPCs and map
 * proto messages to plain view models (bigint → number) so React components and
 * Server Actions stay free of proto types. The frontend never calls team-ai
 * directly — every call goes through the gateway (Rule 1).
 */
import "server-only";

import { makeClients } from "./client.js";
import { getToken } from "./session.js";

// gateway builds request-scoped clients carrying the caller's session token.
function gateway() {
  return makeClients(getToken());
}

export interface ViewProductCard {
  listingId: string;
  title: string;
  price: number;
  currency: string;
  imageUrl: string;
  discountRate: number;
  ratingText: string;
}

export interface ViewAssistantReply {
  replyText: string;
  productCards: ViewProductCard[];
  suggestedFollowups: string[];
}

export interface ViewMagicListing {
  generatedTitle: string;
  generatedDescription: string;
  suggestedCategoryId: string;
  suggestedPriceMin: number;
  suggestedPriceMax: number;
  highlightTags: string[];
}

/** Shopping Assistant (RAG chatbot): question → reply + product cards + followups. */
export async function shoppingAssistant(
  message: string,
  previousContext: string[] = [],
  userId = "",
): Promise<ViewAssistantReply> {
  const res = await gateway().ai.shoppingAssistant({
    userId,
    message,
    previousContext,
  });
  return {
    replyText: res.replyText,
    productCards: res.productCards.map((c) => ({
      listingId: c.listingId,
      title: c.title,
      price: Number(c.price),
      currency: c.currency,
      imageUrl: c.imageUrl,
      discountRate: c.discountRate,
      ratingText: c.ratingText,
    })),
    suggestedFollowups: res.suggestedFollowups,
  };
}

/** Magic Listing ("tạo tin nhanh"): title hint → SEO title/description/category/price. */
export async function magicListing(
  titleHint: string,
  categoryHint = "",
  imageUrl = "",
): Promise<ViewMagicListing> {
  const res = await gateway().ai.magicListing({
    titleHint,
    categoryHint,
    imageUrl,
  });
  return {
    generatedTitle: res.generatedTitle,
    generatedDescription: res.generatedDescription,
    suggestedCategoryId: res.suggestedCategoryId,
    suggestedPriceMin: Number(res.suggestedPriceMin),
    suggestedPriceMax: Number(res.suggestedPriceMax),
    highlightTags: res.highlightTags,
  };
}

/** Chat Copilot: buyer message → up to 3 quick seller replies. */
export async function chatCopilot(
  sellerId: string,
  buyerMessage: string,
  listingId = "",
): Promise<string[]> {
  const res = await gateway().ai.chatCopilot({
    sellerId,
    buyerMessage,
    listingId,
  });
  return res.quickReplies;
}

export interface ViewReviewSummary {
  summary: string;
  pros: string[];
  cons: string[];
  sentiment: string;
}

/** AI review summary: condense a listing's reviews into pros/cons + sentiment. */
export async function summarizeReviews(
  listingId: string,
  reviews: { rating: number; comment: string }[],
): Promise<ViewReviewSummary | null> {
  if (reviews.length === 0) return null;
  try {
    const res = await gateway().ai.summarizeReviews({
      listingId,
      reviews: reviews.map((r) => ({ rating: r.rating, comment: r.comment })),
    });
    return {
      summary: res.summary,
      pros: res.pros,
      cons: res.cons,
      sentiment: res.sentiment,
    };
  } catch {
    return null; // team-ai unavailable — hide the block
  }
}
