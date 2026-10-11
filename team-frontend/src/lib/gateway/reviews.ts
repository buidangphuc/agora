import "server-only";

import { REVIEWS_PAGE_SIZE } from "@/features/listing/pdp";
import type { Review } from "@/generated/platform/engagement/v1/engagement_pb.js";
import { makeClients } from "./client.js";
import { getToken } from "./session.js";

function gateway() {
  return makeClients(getToken());
}

// Public reads: retry once without the bearer if the gateway rejects a stale
// session (it answers Unauthenticated to any invalid token, even on public RPCs).
function publicGateway() {
  return makeClients(getToken(), { anonymousFallback: true });
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
    const res = await publicGateway().engagement.getShopRatingSummary({
      sellerId,
    });
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

/**
 * Sample of a listing's reviews (the server's first page of at most 100, newest
 * first) for features that read many reviews at once, such as the AI summary.
 * The PDP review list pages on the server via `listReviewsPage`.
 */
export const REVIEWS_FETCH_SIZE = 100;

export async function listReviews(
  listingId: string,
  ratingFilter = 0,
): Promise<ViewReview[]> {
  try {
    const res = await publicGateway().engagement.listReviews({
      listingId,
      ratingFilter,
      page: { pageSize: REVIEWS_FETCH_SIZE },
    });
    return res.reviews.map(mapReview);
  } catch {
    return [];
  }
}

export interface ViewReviewsPage {
  reviews: ViewReview[];
  /** Reviews matching the star filter, across all pages. */
  total: number;
  /** The 1-based page actually served (clamped to the last page). */
  page: number;
  pages: number;
}

/**
 * One server page of reviews, newest first. The engagement cursor is the offset
 * of the first review, so page N is `(N - 1) * REVIEWS_PAGE_SIZE`. A page past
 * the end is re-requested as the last page.
 */
export async function listReviewsPage(
  listingId: string,
  opts: { rating?: number; page?: number } = {},
): Promise<ViewReviewsPage> {
  const rating = opts.rating ?? 0;
  const fetchPage = (page: number) =>
    publicGateway().engagement.listReviews({
      listingId,
      ratingFilter: rating,
      page: {
        cursor: String((page - 1) * REVIEWS_PAGE_SIZE),
        pageSize: REVIEWS_PAGE_SIZE,
      },
    });
  try {
    let page = Math.max(1, Math.floor(opts.page ?? 1));
    let res = await fetchPage(page);
    const total = Number(res.page?.total ?? 0);
    const pages = Math.max(1, Math.ceil(total / REVIEWS_PAGE_SIZE));
    if (page > pages) {
      page = pages;
      res = await fetchPage(page);
    }
    return { reviews: res.reviews.map(mapReview), total, page, pages };
  } catch {
    return { reviews: [], total: 0, page: 1, pages: 1 };
  }
}

export async function getListingRatingSummary(
  listingId: string,
): Promise<ViewRatingSummary> {
  try {
    const res = await publicGateway().engagement.getListingRatingSummary({
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
