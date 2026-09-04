import "server-only";

import type { Review } from "@/generated/platform/engagement/v1/engagement_pb.js";
import { makeClients } from "./client.js";
import { getToken } from "./session.js";

function gateway() {
  return makeClients(getToken());
}

export interface ViewReview {
  id: string;
  listingId: string;
  userId: string;
  userName: string;
  orderId: string;
  rating: number;
  comment: string;
  createdAt: string;
  mediaUrls: string[];
  helpfulCount: number;
  verifiedPurchase: boolean;
}

export interface ViewRatingBreakdown {
  star1: number;
  star2: number;
  star3: number;
  star4: number;
  star5: number;
}

export interface ViewRatingSummary {
  listingId: string;
  averageRating: number;
  reviewCount: number;
  breakdown: ViewRatingBreakdown;
}

export interface ViewShopRatingSummary {
  sellerId: string;
  averageRating: number;
  reviewCount: number;
  breakdown: ViewRatingBreakdown;
}

function mapReview(r: Review): ViewReview {
  let createdAt = "";
  if (r.createdAt) {
    createdAt = new Date(Number(r.createdAt.seconds) * 1000).toLocaleDateString(
      "vi-VN",
    );
  }
  return {
    id: r.id,
    listingId: r.listingId,
    userId: r.userId,
    userName: r.userName || "Người mua",
    orderId: r.orderId,
    rating: r.rating,
    comment: r.comment,
    createdAt,
    mediaUrls: r.mediaUrls ?? [],
    helpfulCount: Number(r.helpfulCount),
    verifiedPurchase: r.verifiedPurchase,
  };
}

export async function createReview(
  listingId: string,
  rating: number,
  comment: string,
  orderId?: string,
  mediaUrls: string[] = [],
): Promise<ViewReview> {
  const res = await gateway().engagement.createReview({
    listingId,
    rating,
    comment,
    orderId: orderId ?? "",
    mediaUrls,
  });
  if (!res.review) throw new Error("create review failed");
  return mapReview(res.review);
}

/** Mark a review helpful; returns the new helpful count (team-engagement
 * dedupes per user, so calling twice is a no-op there). */
export async function markReviewHelpful(reviewId: string): Promise<number> {
  const res = await gateway().engagement.markReviewHelpful({ reviewId });
  return Number(res.helpfulCount);
}

export async function getShopRatingSummary(
  sellerId: string,
): Promise<ViewShopRatingSummary> {
  try {
    const res = await gateway().engagement.getShopRatingSummary({ sellerId });
    const b = res.breakdown;
    return {
      sellerId: res.sellerId || sellerId,
      averageRating: res.averageRating || 5.0,
      reviewCount: Number(res.reviewCount),
      breakdown: {
        star1: b?.star1 ?? 0,
        star2: b?.star2 ?? 0,
        star3: b?.star3 ?? 0,
        star4: b?.star4 ?? 0,
        star5: b?.star5 ?? 0,
      },
    };
  } catch {
    return {
      sellerId,
      averageRating: 5.0,
      reviewCount: 0,
      breakdown: { star1: 0, star2: 0, star3: 0, star4: 0, star5: 0 },
    };
  }
}

export async function listReviews(
  listingId: string,
  ratingFilter = 0,
): Promise<ViewReview[]> {
  try {
    const res = await gateway().engagement.listReviews({
      listingId,
      ratingFilter,
    });
    return res.reviews.map(mapReview);
  } catch {
    return [];
  }
}

export async function getListingRatingSummary(
  listingId: string,
): Promise<ViewRatingSummary> {
  try {
    const res = await gateway().engagement.getListingRatingSummary({
      listingId,
    });
    const b = res.breakdown;
    return {
      listingId: res.listingId,
      averageRating: res.averageRating || 5.0,
      reviewCount: Number(res.reviewCount),
      breakdown: {
        star1: b?.star1 ?? 0,
        star2: b?.star2 ?? 0,
        star3: b?.star3 ?? 0,
        star4: b?.star4 ?? 0,
        star5: b?.star5 ?? 0,
      },
    };
  } catch {
    return {
      listingId,
      averageRating: 5.0,
      reviewCount: 0,
      breakdown: { star1: 0, star2: 0, star3: 0, star4: 0, star5: 0 },
    };
  }
}
