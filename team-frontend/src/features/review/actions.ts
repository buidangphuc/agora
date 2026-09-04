"use server";

import { revalidatePath } from "next/cache";

import { createReview, markReviewHelpful } from "@/lib/gateway/reviews";

export async function createReviewAction(
  listingId: string,
  rating: number,
  comment: string,
  orderId?: string,
  mediaUrls: string[] = [],
): Promise<{ ok: boolean; message?: string }> {
  try {
    await createReview(listingId, rating, comment, orderId, mediaUrls);
    revalidatePath(`/listing/${listingId}`);
    revalidatePath("/account/orders");
    return { ok: true, message: "Đánh giá sản phẩm thành công!" };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Gửi đánh giá thất bại.",
    };
  }
}

export async function markReviewHelpfulAction(
  reviewId: string,
): Promise<{ ok: boolean; helpfulCount?: number; message?: string }> {
  try {
    const helpfulCount = await markReviewHelpful(reviewId);
    return { ok: true, helpfulCount };
  } catch (err: unknown) {
    return {
      ok: false,
      message:
        err instanceof Error
          ? err.message
          : "Không ghi nhận được lượt hữu ích.",
    };
  }
}
