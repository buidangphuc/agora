"use client";

import { useState } from "react";

import { createReviewAction } from "./actions";

export function ReviewModal({
  listingId,
  orderId,
  productTitle,
  onClose,
  onSuccess,
}: {
  listingId: string;
  orderId?: string;
  productTitle?: string;
  onClose: () => void;
  onSuccess?: () => void;
}) {
  const [rating, setRating] = useState(5);
  const [comment, setComment] = useState("");
  const [mediaInput, setMediaInput] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  // Photo URLs, one per line — uses the same stored media URLs the listing
  // gallery renders (no new upload widget introduced here).
  const mediaUrls = mediaInput
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!comment.trim()) {
      setError("Vui lòng nhập nội dung đánh giá.");
      return;
    }
    setSubmitting(true);
    setError("");
    try {
      const res = await createReviewAction(
        listingId,
        rating,
        comment.trim(),
        orderId,
        mediaUrls,
      );
      if (!res.ok) {
        setError(res.message || "Gửi đánh giá thất bại.");
        return;
      }
      if (onSuccess) onSuccess();
      onClose();
    } catch {
      setError("Có lỗi xảy ra khi gửi đánh giá.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4 backdrop-blur-xs">
      <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-xl">
        <div className="flex items-center justify-between border-b pb-3">
          <h2 className="text-base font-bold text-gray-900">
            ⭐ Đánh giá sản phẩm
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600"
          >
            ✕
          </button>
        </div>

        {productTitle && (
          <p className="mt-3 line-clamp-1 text-xs text-gray-500">
            Sản phẩm: <strong className="text-gray-800">{productTitle}</strong>
          </p>
        )}

        <form onSubmit={handleSubmit} className="mt-4 space-y-4">
          {/* Star Selector */}
          <div>
            <div className="block text-xs font-semibold text-gray-700">
              Chất lượng sản phẩm
            </div>
            <div className="mt-2 flex items-center gap-2">
              {[1, 2, 3, 4, 5].map((star) => (
                <button
                  key={star}
                  type="button"
                  onClick={() => setRating(star)}
                  className={`text-2xl transition hover:scale-110 ${
                    star <= rating ? "text-amber-400" : "text-gray-200"
                  }`}
                >
                  ★
                </button>
              ))}
              <span className="ml-2 text-xs font-medium text-amber-600">
                {rating === 5
                  ? "Tuyệt vời (5/5)"
                  : rating === 4
                    ? "Hài lòng (4/5)"
                    : rating === 3
                      ? "Bình thường (3/5)"
                      : rating === 2
                        ? "Không hài lòng (2/5)"
                        : "Rất tệ (1/5)"}
              </span>
            </div>
          </div>

          {/* Comment */}
          <div>
            <label
              htmlFor="review-comment-input"
              className="block text-xs font-semibold text-gray-700"
            >
              Nhận xét chi tiết
            </label>
            <textarea
              id="review-comment-input"
              required
              rows={4}
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              placeholder="Hãy chia sẻ trải nghiệm về sản phẩm, chất lượng đóng gói và thời gian giao hàng nhé..."
              className="mt-1.5 w-full rounded-lg border border-gray-300 p-2.5 text-xs text-gray-900 focus:border-brand focus:outline-hidden"
            />
          </div>

          {/* Photos (optional) */}
          <div>
            <label
              htmlFor="review-media-input"
              className="block text-xs font-semibold text-gray-700"
            >
              Ảnh đánh giá (tùy chọn)
            </label>
            <textarea
              id="review-media-input"
              rows={2}
              value={mediaInput}
              onChange={(e) => setMediaInput(e.target.value)}
              placeholder="Dán link ảnh, mỗi ảnh một dòng…"
              className="mt-1.5 w-full rounded-lg border border-gray-300 p-2.5 text-xs text-gray-900 focus:border-brand focus:outline-hidden"
            />
            {mediaUrls.length > 0 && (
              <div className="mt-2 flex flex-wrap gap-2">
                {mediaUrls.map((url) => (
                  <div
                    key={url}
                    className="h-12 w-12 overflow-hidden rounded-xs border bg-gray-50"
                  >
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={url}
                      alt="preview"
                      className="h-full w-full object-cover"
                    />
                  </div>
                ))}
              </div>
            )}
          </div>

          {error && <p className="text-xs text-red-600">{error}</p>}

          <div className="flex justify-end gap-2 border-t pt-3">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border px-4 py-2 text-xs font-medium text-gray-700 hover:bg-gray-50"
            >
              Hủy
            </button>
            <button
              type="submit"
              disabled={submitting}
              className="rounded-lg bg-brand px-5 py-2 text-xs font-semibold text-white shadow-xs hover:bg-brand-dark disabled:opacity-60"
            >
              {submitting ? "Đang gửi..." : "Hoàn thành"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
