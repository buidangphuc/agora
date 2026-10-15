"use server";

import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";

import { type ActionResult, fail, ok } from "@/lib/action-result";
import {
  createReview,
  hideReview,
  markReviewHelpful,
} from "@/lib/gateway/reviews";

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof Error && err.message !== "" ? err.message : fallback;
}

const SESSION_EXPIRED = "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.";
const ORDERS_HINT = "Vui lòng đánh giá từ trang Đơn hàng của tôi.";

function connectCode(err: unknown): Code | undefined {
  return err instanceof ConnectError ? err.code : undefined;
}

function createErrorMessage(err: unknown): string {
  switch (connectCode(err)) {
    case Code.AlreadyExists:
      return "Bạn đã đánh giá sản phẩm này cho đơn hàng này.";
    case Code.FailedPrecondition:
      return "Chỉ có thể đánh giá sản phẩm trong đơn hàng đã hoàn thành của chính bạn.";
    case Code.Unavailable:
      return "Chưa xác minh được đơn hàng của bạn. Vui lòng thử lại sau.";
    case Code.InvalidArgument:
      return ORDERS_HINT;
    case Code.Unauthenticated:
      return SESSION_EXPIRED;
    default:
      return "Gửi đánh giá thất bại.";
  }
}

function hideErrorMessage(err: unknown): string {
  switch (connectCode(err)) {
    case Code.PermissionDenied:
      return "Bạn không có quyền ẩn đánh giá.";
    case Code.NotFound:
      return "Đánh giá không còn tồn tại.";
    case Code.Unavailable:
      return "Chưa ẩn được đánh giá. Vui lòng thử lại sau.";
    case Code.Unauthenticated:
      return SESSION_EXPIRED;
    default:
      return "Ẩn đánh giá thất bại.";
  }
}

export async function createReviewAction(
  listingId: string,
  rating: number,
  comment: string,
  orderId?: string,
  mediaUrls: string[] = [],
): Promise<ActionResult> {
  if (!orderId) return fail(ORDERS_HINT);
  try {
    await createReview(listingId, rating, comment, orderId, mediaUrls);
    revalidatePath(`/listing/${listingId}`);
    revalidatePath("/account/orders");
    return ok();
  } catch (err: unknown) {
    return fail(createErrorMessage(err));
  }
}

export async function hideReviewAction(
  reviewId: string,
  listingId: string,
  reason: string,
): Promise<ActionResult> {
  const trimmed = reason.trim();
  if (trimmed.length < 1 || trimmed.length > 500) {
    return fail("Lý do ẩn phải từ 1 đến 500 ký tự.");
  }
  try {
    await hideReview(reviewId, trimmed);
    revalidatePath(`/listing/${listingId}`);
    return ok();
  } catch (err: unknown) {
    return fail(hideErrorMessage(err));
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
