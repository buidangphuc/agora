"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import type { ViewRatingSummary, ViewReview } from "@/lib/gateway/reviews";
import { ReviewModal } from "./ReviewModal";
import { markReviewHelpfulAction } from "./actions";

function ReviewItem({ review }: { review: ViewReview }) {
  const [helpful, setHelpful] = useState(review.helpfulCount);
  const [voted, setVoted] = useState(false);
  const [pending, start] = useTransition();
  const toast = useToast();

  function vote() {
    if (voted || pending) return;
    // Optimistic bump; team-engagement dedupes per user so the count is
    // authoritative on the response.
    setVoted(true);
    setHelpful((c) => c + 1);
    start(async () => {
      const res = await markReviewHelpfulAction(review.id);
      if (res.ok && typeof res.helpfulCount === "number") {
        setHelpful(res.helpfulCount);
      } else {
        setVoted(false);
        setHelpful(review.helpfulCount);
        toast.error(res.message || "Không ghi nhận được lượt hữu ích.");
      }
    });
  }

  return (
    <div data-testid="review-item" className="py-4 space-y-1.5">
      <div className="flex items-center gap-2">
        <div className="grid h-7 w-7 place-items-center rounded-full bg-gray-100 text-xs font-bold text-gray-600">
          {review.userName.charAt(0).toUpperCase()}
        </div>
        <div>
          <span className="text-xs font-semibold text-gray-800">
            {review.userName}
          </span>
          {review.verifiedPurchase && (
            <span
              data-testid="verified-purchase"
              className="ml-2 inline-flex items-center gap-0.5 rounded-2xs bg-emerald-50 px-1.5 py-0.5 text-[10px] font-semibold text-emerald-700"
            >
              ✓ Đã mua hàng
            </span>
          )}
          <span className="ml-2 text-[10px] text-gray-400">
            {review.createdAt}
          </span>
        </div>
      </div>

      <div className="flex text-xs text-amber-400">
        {"★".repeat(review.rating)}
        {"☆".repeat(5 - review.rating)}
      </div>

      <p className="text-xs leading-relaxed text-gray-700">{review.comment}</p>

      {/* Review photos */}
      {review.mediaUrls.length > 0 && (
        <div className="flex flex-wrap gap-2 pt-1">
          {review.mediaUrls.map((url) => (
            <div
              key={url}
              className="h-16 w-16 shrink-0 overflow-hidden rounded-xs border bg-gray-50"
            >
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={url}
                alt="Ảnh đánh giá"
                className="h-full w-full object-cover"
              />
            </div>
          ))}
        </div>
      )}

      {/* Helpful vote */}
      <div className="pt-1">
        <button
          type="button"
          data-testid="review-helpful"
          onClick={vote}
          disabled={voted || pending}
          className={`inline-flex items-center gap-1 rounded-full border px-3 py-1 text-[11px] font-medium transition ${
            voted
              ? "border-brand bg-orange-50 text-brand"
              : "border-gray-200 text-gray-600 hover:border-brand hover:text-brand"
          }`}
        >
          <span>👍 Hữu ích</span>
          <span className="font-bold">({helpful})</span>
        </button>
      </div>
    </div>
  );
}

export function ReviewSection({
  listingId,
  productTitle,
  initialSummary,
  initialReviews,
}: {
  listingId: string;
  productTitle?: string;
  initialSummary: ViewRatingSummary;
  initialReviews: ViewReview[];
}) {
  const [reviews] = useState<ViewReview[]>(initialReviews);
  const [summary] = useState<ViewRatingSummary>(initialSummary);
  const [ratingFilter, setRatingFilter] = useState<number>(0);
  const [modalOpen, setModalOpen] = useState(false);

  const filteredReviews =
    ratingFilter === 0
      ? reviews
      : reviews.filter((r) => r.rating === ratingFilter);

  return (
    <div className="mt-8 rounded-2xl border bg-white p-6 shadow-xs">
      <div className="flex items-center justify-between border-b pb-4">
        <div>
          <h2 className="text-lg font-bold text-gray-900">ĐÁNH GIÁ SẢN PHẨM</h2>
          <p className="mt-0.5 text-xs text-gray-500">
            Nhận xét thực tế từ người mua hàng đã trải nghiệm
          </p>
        </div>

        <button
          type="button"
          onClick={() => setModalOpen(true)}
          className="rounded-lg bg-orange-50 px-4 py-2 text-xs font-semibold text-brand transition hover:bg-orange-100"
        >
          ✍️ Viết đánh giá
        </button>
      </div>

      {/* Summary Box */}
      <div className="mt-5 flex flex-col gap-6 rounded-xl bg-orange-50/40 p-5 sm:flex-row sm:items-center">
        <div className="text-center sm:border-r sm:pr-8">
          <div className="text-3xl font-extrabold text-brand">
            {summary.averageRating.toFixed(1)}{" "}
            <span className="text-base font-normal text-gray-500">/ 5</span>
          </div>
          <div className="mt-1 flex justify-center text-amber-400">
            {"★".repeat(Math.round(summary.averageRating))}
            {"☆".repeat(5 - Math.round(summary.averageRating))}
          </div>
          <p className="mt-1 text-xs text-gray-500">
            {summary.reviewCount} đánh giá
          </p>
        </div>

        {/* Filter Pills */}
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            onClick={() => setRatingFilter(0)}
            className={`rounded-lg px-3.5 py-1.5 text-xs font-medium transition ${
              ratingFilter === 0
                ? "bg-brand text-white shadow-xs"
                : "border bg-white text-gray-700 hover:border-gray-300"
            }`}
          >
            Tất cả ({reviews.length})
          </button>
          {[5, 4, 3, 2, 1].map((star) => {
            const count =
              star === 5
                ? summary.breakdown.star5
                : star === 4
                  ? summary.breakdown.star4
                  : star === 3
                    ? summary.breakdown.star3
                    : star === 2
                      ? summary.breakdown.star2
                      : summary.breakdown.star1;
            return (
              <button
                key={star}
                type="button"
                onClick={() => setRatingFilter(star)}
                className={`rounded-lg px-3.5 py-1.5 text-xs font-medium transition ${
                  ratingFilter === star
                    ? "bg-brand text-white shadow-xs"
                    : "border bg-white text-gray-700 hover:border-gray-300"
                }`}
              >
                {star} Sao ({count})
              </button>
            );
          })}
        </div>
      </div>

      {/* Reviews List */}
      <div className="mt-6 divide-y">
        {filteredReviews.length === 0 ? (
          <div className="py-8 text-center text-xs text-gray-400">
            Chưa có đánh giá nào cho phân loại này.
          </div>
        ) : (
          filteredReviews.map((r) => <ReviewItem key={r.id} review={r} />)
        )}
      </div>

      {modalOpen && (
        <ReviewModal
          listingId={listingId}
          productTitle={productTitle}
          onClose={() => setModalOpen(false)}
          onSuccess={() => {
            // refresh handled by revalidatePath in the server action
          }}
        />
      )}
    </div>
  );
}
