"use client";

import React, { useState } from "react";

import { Button } from "@/components/ui/Button";
import { FormItem } from "@/components/ui/FormItem";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { usePendingAction } from "@/features/order/usePendingAction";
import type { ViewDispute } from "@/lib/gateway/disputes";
import { createDisputeAction } from "./actions";

const MAX_REASON = 1000;

const textareaClass =
  "block w-full rounded-lg border border-border-subtle bg-surface-card px-3.5 py-2 text-sm text-text-primary shadow-2xs transition duration-150 placeholder:text-text-disabled focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring disabled:cursor-not-allowed disabled:opacity-50";

export function EscalateDisputeModal({
  isOpen,
  onClose,
  orderId,
  sellerId,
  returnId,
  onCreated,
}: {
  isOpen: boolean;
  onClose: () => void;
  orderId: string;
  sellerId: string;
  returnId: string;
  onCreated: (dispute: ViewDispute) => void;
}) {
  const toast = useToast();
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | undefined>();
  const [pending, run] = usePendingAction();

  function close() {
    if (!pending) onClose();
  }

  function submit() {
    if (pending) return;
    const trimmed = reason.trim();
    if (!trimmed) {
      setError("Vui lòng nhập lý do khiếu nại.");
      return;
    }
    if (trimmed.length > MAX_REASON) {
      setError("Lý do tối đa 1000 ký tự.");
      return;
    }
    setError(undefined);
    void run(async () => {
      const res = await createDisputeAction({
        orderId,
        defendantId: sellerId,
        returnId,
        reason: trimmed,
      });
      if (res.ok && res.data) {
        toast.success("Đã gửi khiếu nại lên Agora.");
        onCreated(res.data);
        onClose();
      } else if (!res.ok) {
        setError(res.error);
      }
    });
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={close}
      size="sm"
      title="Khiếu nại lên Agora"
      description="Mô tả vì sao bạn không đồng ý với quyết định của người bán."
      footer={
        <>
          <Button variant="outline" disabled={pending} onClick={close}>
            Đóng
          </Button>
          <Button
            data-testid="dispute-submit"
            isLoading={pending}
            onClick={submit}
          >
            Gửi khiếu nại
          </Button>
        </>
      }
    >
      <FormItem
        label="Lý do khiếu nại"
        required
        status={error ? "error" : undefined}
        help={error}
      >
        <textarea
          data-testid="dispute-reason"
          rows={4}
          value={reason}
          disabled={pending}
          onChange={(e) => setReason(e.target.value)}
          className={textareaClass}
        />
      </FormItem>
    </Modal>
  );
}
