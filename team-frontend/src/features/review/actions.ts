"use server";

import { revalidatePath } from "next/cache";

import { type ActionResult, fail, ok } from "@/lib/action-result";
import { createReview, markReviewHelpful } from "@/lib/gateway/reviews";

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof Error && err.message !== "" ? err.message : fallback;
}

export async function createReviewAction(
  listingId: string,
  rating: number,
  comment: string,
  orderId?: string,
  mediaUrls: string[] = [],
): Promise<ActionResult> {
  try {
    await createReview(listingId, rating, comment, orderId, mediaUrls);
    revalidatePath(`/listing/${listingId}`);
    revalidatePath("/account/orders");
    return ok();
  } catch (err: unknown) {
    return fail(errorMessage(err, "Gửi đánh giá thất bại."));
  }
}

export async function markReviewHelpfulAction(
  reviewId: string,
  listingId: string,
): Promise<ActionResult<{ helpfulCount: number }>> {
  try {
    const helpfulCount = await markReviewHelpful(reviewId);
    revalidatePath(`/listing/${listingId}`);
    return ok({ helpfulCount });
  } catch (err: unknown) {
    return fail(errorMessage(err, "Không ghi nhận được lượt hữu ích."));
  }
}
