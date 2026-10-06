"use client";

import React, { useState } from "react";

import { Button } from "@/components/ui/Button";
import { FormItem } from "@/components/ui/FormItem";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";
import { useToast } from "@/components/ui/ToastProvider";
import type { ViewOrderReturn } from "@/lib/gateway/orders";
import { createReturnRequestAction } from "./actions";
import { usePendingAction } from "./usePendingAction";

export const RETURN_REASONS = [
  { value: "changed_mind", label: "Tôi đổi ý, không muốn nhận hàng nữa" },
  { value: "defective", label: "Sản phẩm bị lỗi / hư hỏng" },
  { value: "wrong_item", label: "Giao sai sản phẩm hoặc phân loại" },
  { value: "other", label: "Lý do khác" },
];

const textareaClass =
  "block w-full rounded-lg border border-border-subtle bg-surface-card px-3.5 py-2 text-sm text-text-primary shadow-2xs transition duration-150 placeholder:text-text-disabled focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring disabled:cursor-not-allowed disabled:opacity-50";

interface Errors {
  reason?: string;
  detail?: string;
  amount?: string;
}

export function validateReturnForm(
  reasonKey: string,
  detail: string,
  amount: number,
  orderTotal: number,
): Errors {
  const errors: Errors = {};
  if (!reasonKey) errors.reason = "Vui lòng chọn lý do trả hàng.";
  if (reasonKey === "other" && !detail.trim()) {
    errors.detail = "Vui lòng mô tả lý do.";
  }
  if (!(amount > 0)) {
    errors.amount = "Số tiền hoàn phải lớn hơn 0.";
  } else if (amount > orderTotal) {
    errors.amount = "Số tiền hoàn không được vượt quá tổng đơn hàng.";
  }
  return errors;
}

/** RMA form in a Modal: validation inline, pending state, toasts. */
export function ReturnRequestModal({
  isOpen,
  onClose,
  orderId,
  orderTotal,
  onCreated,
}: {
  isOpen: boolean;
  onClose: () => void;
  orderId: string;
  orderTotal: number;
  onCreated: (ret: ViewOrderReturn) => void;
}) {
  const toast = useToast();
  const [reasonKey, setReasonKey] = useState("");
  const [detail, setDetail] = useState("");
  const [amount, setAmount] = useState(String(orderTotal));
  const [errors, setErrors] = useState<Errors>({});
  const [pending, run] = usePendingAction();

  function close() {
    if (!pending) onClose();
  }

  function submit() {
    if (pending) return;
    const value = Number(amount);
    const found = validateReturnForm(reasonKey, detail, value, orderTotal);
    setErrors(found);
    if (Object.keys(found).length > 0) return;

    const reason =
      reasonKey === "other"
        ? detail.trim()
        : (RETURN_REASONS.find((r) => r.value === reasonKey)?.label ??
          reasonKey);
    void run(async () => {
      const res = await createReturnRequestAction(orderId, reason, value);
      if (res.ok && res.data) {
        toast.success("Đã gửi yêu cầu trả hàng / hoàn tiền.");
        onCreated(res.data);
        onClose();
      } else if (!res.ok) {
        toast.error(res.error);
      }
    });
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={close}
      size="sm"
      title="Yêu cầu trả hàng / hoàn tiền"
      description="Chọn lý do và số tiền muốn hoàn. Hoàn tiền là mô phỏng (demo)."
      footer={
        <>
          <Button variant="outline" disabled={pending} onClick={close}>
            Đóng
          </Button>
          <Button
            data-testid="return-submit"
            isLoading={pending}
            onClick={submit}
          >
            Gửi yêu cầu trả hàng
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <FormItem
          label="Lý do trả hàng"
          required
          status={errors.reason ? "error" : undefined}
          help={errors.reason}
        >
          <Select
            data-testid="return-reason"
            placeholder="Chọn lý do"
            value={reasonKey}
            disabled={pending}
            options={RETURN_REASONS}
            onChange={(e) => setReasonKey(e.target.value)}
          />
        </FormItem>

        {reasonKey === "other" && (
          <FormItem
            label="Mô tả lý do"
            required
            status={errors.detail ? "error" : undefined}
            help={errors.detail}
          >
            <textarea
              data-testid="return-reason-detail"
              rows={3}
              value={detail}
              disabled={pending}
              onChange={(e) => setDetail(e.target.value)}
              className={textareaClass}
              placeholder="Sản phẩm bị lỗi, giao sai mẫu…"
            />
          </FormItem>
        )}

        <FormItem
          label="Số tiền hoàn (₫)"
          required
          status={errors.amount ? "error" : undefined}
          help={
            errors.amount ?? `Tối đa ${orderTotal.toLocaleString("vi-VN")}₫`
          }
        >
          <Input
            data-testid="return-amount"
            type="number"
            min={0}
            max={orderTotal}
            value={amount}
            disabled={pending}
            onChange={(e) => setAmount(e.target.value)}
          />
        </FormItem>
      </div>
    </Modal>
  );
}
