"use client";

import { useId, useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Image } from "@/components/ui/Image";
import { Modal } from "@/components/ui/Modal";
import { Rate } from "@/components/ui/Rate";
import { useToast } from "@/components/ui/ToastProvider";
import { createReviewAction } from "./actions";

const RATING_LABELS: Record<number, string> = {
  5: "Tuyệt vời (5/5)",
  4: "Hài lòng (4/5)",
  3: "Bình thường (3/5)",
  2: "Không hài lòng (2/5)",
  1: "Rất tệ (1/5)",
};

const fieldClass =
  "mt-1.5 w-full rounded-lg border border-border-strong bg-surface-card p-2.5 text-sm text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring disabled:opacity-50 disabled:cursor-not-allowed";

/**
 * "Viết đánh giá": a Modal with a `Rate` input, a comment and optional photo
 * links. Submit shows `isLoading` and disables the controls, then a success
 * toast and close; a failure keeps the modal open with an inline `Alert`.
 */
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
  const formId = useId();
  const commentId = useId();
  const mediaId = useId();
  const toast = useToast();
  const [rating, setRating] = useState(5);
  const [comment, setComment] = useState("");
  const [mediaInput, setMediaInput] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  // Photo URLs, one per line: the same stored media URLs the listing gallery
  // renders (no upload widget is introduced here).
  const mediaUrls = mediaInput
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (submitting) return;
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
        setError(res.error);
        return;
      }
      toast.success("Đánh giá sản phẩm thành công!");
      onSuccess?.();
      onClose();
    } catch {
      setError("Có lỗi xảy ra khi gửi đánh giá.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Modal
      isOpen
      onClose={() => {
        if (!submitting) onClose();
      }}
      title="Đánh giá sản phẩm"
      description={productTitle ? `Sản phẩm: ${productTitle}` : undefined}
      footer={
        <>
          <Button
            variant="outline"
            size="md"
            disabled={submitting}
            onClick={onClose}
          >
            Hủy
          </Button>
          <Button
            type="submit"
            form={formId}
            variant="primary"
            size="md"
            isLoading={submitting}
          >
            Hoàn thành
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={handleSubmit} className="space-y-4">
        <div>
          <div className="text-sm font-medium text-text-primary">
            Chất lượng sản phẩm
          </div>
          <div className="mt-2 flex items-center gap-3">
            <Rate
              value={rating}
              onChange={setRating}
              size="lg"
              disabled={submitting}
              label="Chất lượng sản phẩm"
            />
            <span className="text-sm font-medium text-text-secondary">
              {RATING_LABELS[rating]}
            </span>
          </div>
        </div>

        <div>
          <label
            htmlFor={commentId}
            className="block text-sm font-medium text-text-primary"
          >
            Nhận xét chi tiết
          </label>
          <textarea
            id={commentId}
            rows={4}
            value={comment}
            disabled={submitting}
            onChange={(e) => setComment(e.target.value)}
            placeholder="Hãy chia sẻ trải nghiệm về sản phẩm, chất lượng đóng gói và thời gian giao hàng nhé..."
            className={fieldClass}
          />
        </div>

        <div>
          <label
            htmlFor={mediaId}
            className="block text-sm font-medium text-text-primary"
          >
            Ảnh đánh giá (tùy chọn)
          </label>
          <textarea
            id={mediaId}
            rows={2}
            value={mediaInput}
            disabled={submitting}
            onChange={(e) => setMediaInput(e.target.value)}
            placeholder="Dán link ảnh, mỗi ảnh một dòng…"
            className={fieldClass}
          />
          {mediaUrls.length > 0 && (
            <div className="mt-2 flex flex-wrap gap-2">
              {mediaUrls.map((url) => (
                <div key={url} className="h-12 w-12 shrink-0">
                  <Image src={url} alt="Xem trước ảnh" aspect="square" />
                </div>
              ))}
            </div>
          )}
        </div>

        {error && <Alert type="error" title={error} />}
      </form>
    </Modal>
  );
}
