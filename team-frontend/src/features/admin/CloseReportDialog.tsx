"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { usePending } from "@/features/seller/usePending";

import { closeReportAction } from "./reportActions";

const MAX_NOTE = 500;
const fieldClass =
  "w-full rounded-md border border-border-strong bg-surface-card px-3 py-1.5 text-sm text-text-primary";

export function CloseReportDialog({
  reportId,
  isOpen,
  onClose,
}: {
  reportId: string;
  isOpen: boolean;
  onClose: () => void;
}) {
  const [outcome, setOutcome] = useState<"actioned" | "dismissed">("actioned");
  const [note, setNote] = useState("");
  const [fieldError, setFieldError] = useState("");
  const [error, setError] = useState("");
  const { pending, run } = usePending();
  const toast = useToast();

  function close() {
    if (pending) return;
    setError("");
    setFieldError("");
    onClose();
  }

  async function onConfirm() {
    if (pending) return;
    setError("");
    if (!note.trim()) return setFieldError("Vui lòng nhập ghi chú xử lý.");
    setFieldError("");
    const res = await run(() =>
      closeReportAction(reportId, outcome, note.trim()),
    );
    if (res.ok) {
      toast.success(res.message);
      onClose();
    } else {
      setError(res.error);
    }
  }

  const id = `close-report-${reportId}`;
  return (
    <Modal
      isOpen={isOpen}
      onClose={close}
      size="sm"
      title="Đóng báo cáo"
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
        <div className="flex flex-col gap-1">
          <label htmlFor={`${id}-outcome`} className="text-xs">
            Kết quả
          </label>
          <select
            id={`${id}-outcome`}
            value={outcome}
            onChange={(e) =>
              setOutcome(e.target.value as "actioned" | "dismissed")
            }
            className={fieldClass}
          >
            <option value="actioned">Đã xử lý</option>
            <option value="dismissed">Bác bỏ</option>
          </select>
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor={`${id}-note`} className="text-xs">
            Ghi chú xử lý (tối đa 500 ký tự)
          </label>
          <textarea
            id={`${id}-note`}
            value={note}
            maxLength={MAX_NOTE}
            onChange={(e) => setNote(e.target.value.slice(0, MAX_NOTE))}
            className={fieldClass}
          />
        </div>
        {fieldError ? (
          <p className="text-sm text-danger">{fieldError}</p>
        ) : null}
        {error ? <Alert type="error" description={error} /> : null}
      </div>
    </Modal>
  );
}
