"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { usePending } from "@/features/seller/usePending";

import {
  type ModerateListingActionResult,
  moderateListingAction,
} from "./moderationActions";

export type FixedDecision = "reject" | "ban";

const REASONS: [string, string][] = [
  ["prohibited_item", "Hàng cấm"],
  ["counterfeit", "Hàng giả"],
  ["misleading", "Thông tin gây hiểu lầm"],
  ["inappropriate", "Nội dung không phù hợp"],
  ["duplicate", "Tin đăng trùng lặp"],
  ["other", "Khác"],
];

const MAX_NOTE = 500;
const fieldClass =
  "w-full rounded-md border border-border-strong bg-surface-card px-3 py-1.5 text-sm text-text-primary";

export function ModerateDialog({
  listingId,
  decision,
  isOpen,
  onClose,
  onResult,
}: {
  listingId: string;
  decision?: FixedDecision;
  isOpen: boolean;
  onClose: () => void;
  onResult?: (res: ModerateListingActionResult) => void;
}) {
  const [picked, setPicked] = useState("");
  const [reason, setReason] = useState("");
  const [note, setNote] = useState("");
  const [fieldError, setFieldError] = useState("");
  const [error, setError] = useState("");
  const { pending, run } = usePending();
  const toast = useToast();
  const chosen = decision ?? picked;

  function close() {
    if (pending) return;
    setError("");
    setFieldError("");
    onClose();
  }

  async function onConfirm() {
    if (pending) return;
    setError("");
    if (!chosen) return setFieldError("Vui lòng chọn quyết định.");
    if (!reason) return setFieldError("Vui lòng chọn lý do.");
    setFieldError("");
    const res = await run(() =>
      moderateListingAction(
        listingId,
        chosen as FixedDecision,
        reason,
        note.trim(),
      ),
    );
    onResult?.(res);
    if (res.ok) {
      toast.success(res.message);
      onClose();
    } else {
      setError(res.error);
    }
  }

  const id = `moderate-${listingId}`;
  return (
    <Modal
      isOpen={isOpen}
      onClose={close}
      size="sm"
      title="Xử lý tin đăng"
      footer={
        <>
          <Button variant="outline" onClick={close} disabled={pending}>
            Huỷ
          </Button>
          <Button onClick={onConfirm} isLoading={pending} disabled={pending}>
            Xác nhận
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        {decision ? null : (
          <div className="flex flex-col gap-1">
            <label htmlFor={`${id}-decision`} className="text-xs">
              Quyết định
            </label>
            <select
              id={`${id}-decision`}
              value={picked}
              onChange={(e) => setPicked(e.target.value)}
              className={fieldClass}
            >
              <option value="">Chọn quyết định</option>
              <option value="reject">Từ chối</option>
              <option value="ban">Khóa</option>
            </select>
          </div>
        )}
        <div className="flex flex-col gap-1">
          <label htmlFor={`${id}-reason`} className="text-xs">
            Lý do
          </label>
          <select
            id={`${id}-reason`}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            className={fieldClass}
          >
            <option value="">Chọn lý do</option>
            {REASONS.map(([v, label]) => (
              <option key={v} value={v}>
                {label}
              </option>
            ))}
          </select>
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor={`${id}-note`} className="text-xs">
            Ghi chú (tối đa 500 ký tự)
          </label>
          <textarea
            id={`${id}-note`}
            value={note}
            maxLength={MAX_NOTE}
            onChange={(e) => setNote(e.target.value.slice(0, MAX_NOTE))}
            className={fieldClass}
          />
          <span className="text-xs text-text-secondary">
            {note.length}/{MAX_NOTE}
          </span>
        </div>
        {fieldError ? (
          <p className="text-sm text-danger">{fieldError}</p>
        ) : null}
        {error ? <Alert type="error" description={error} /> : null}
      </div>
    </Modal>
  );
}
