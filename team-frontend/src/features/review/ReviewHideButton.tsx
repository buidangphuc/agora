"use client";

import { useId, useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { hideReviewAction } from "./actions";

const MAX_REASON = 500;

/** Admin-only "Ẩn": opens a dialog asking for the reason, then hides the review. */
export function ReviewHideButton({
  reviewId,
  listingId,
}: {
  reviewId: string;
  listingId: string;
}) {
  const formId = useId();
  const reasonId = useId();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  function close() {
    setOpen(false);
    setReason("");
    setError("");
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (pending || reason.trim() === "") return;
    setPending(true);
    setError("");
    try {
      const res = await hideReviewAction(reviewId, listingId, reason);
      if (!res.ok) {
        setError(res.error);
        return;
      }
      toast.success("Đã ẩn đánh giá.");
      close();
    } catch {
      setError("Ẩn đánh giá thất bại.");
    } finally {
      setPending(false);
    }
  }

  return (
    <>
      <Button
        data-testid="hide-review"
        variant="outline"
        size="sm"
        onClick={() => setOpen(true)}
      >
        Ẩn
      </Button>
      <Modal
        isOpen={open}
        onClose={() => {
          if (!pending) close();
        }}
        title="Ẩn đánh giá"
        footer={
          <>
            <Button
              variant="outline"
              size="md"
              disabled={pending}
              onClick={close}
            >
              Hủy
            </Button>
            <Button
              type="submit"
              form={formId}
              variant="primary"
              size="md"
              isLoading={pending}
              disabled={reason.trim() === ""}
            >
              Ẩn đánh giá
            </Button>
          </>
        }
      >
        <form id={formId} onSubmit={handleSubmit} className="space-y-3">
          <label
            htmlFor={reasonId}
            className="block text-sm font-medium text-text-primary"
          >
            Lý do ẩn
          </label>
          <textarea
            id={reasonId}
            rows={4}
            maxLength={MAX_REASON}
            value={reason}
            disabled={pending}
            onChange={(e) => setReason(e.target.value)}
            className="w-full rounded-xl border border-border-strong bg-surface-card px-4 py-3 text-sm text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring disabled:cursor-not-allowed disabled:opacity-50"
          />
          <div className="text-right text-xs text-text-secondary">
            {reason.length}/{MAX_REASON}
          </div>
          {error && <Alert type="error" title={error} />}
        </form>
      </Modal>
    </>
  );
}
