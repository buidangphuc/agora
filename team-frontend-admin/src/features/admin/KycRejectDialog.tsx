"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { usePending } from "@/lib/usePending";

import { reviewKycAction } from "./kycActions";
import { KYC_REASON_CODES, KYC_REASON_LABELS } from "./kycReasons";

const fieldClass =
  "w-full rounded-md border border-border-strong bg-surface-card px-3 py-1.5 text-sm text-text-primary";

export function KycRejectDialog({
  submissionId,
  isOpen,
  onClose,
}: {
  submissionId: string;
  isOpen: boolean;
  onClose: () => void;
}) {
  const [reason, setReason] = useState("");
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
    if (!reason) return setFieldError("Vui lòng chọn lý do.");
    setFieldError("");
    const fd = new FormData();
    fd.set("submissionId", submissionId);
    fd.set("decision", "reject");
    fd.set("reason", reason);
    const res = await run(() => reviewKycAction(fd));
    if (res.ok) {
      toast.success(res.message);
      onClose();
    } else {
      setError(res.error);
    }
  }

  const id = `kyc-reject-${submissionId}`;
  return (
    <Modal
      isOpen={isOpen}
      onClose={close}
      size="sm"
      title="Từ chối hồ sơ xác minh"
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
            {KYC_REASON_CODES.map((c) => (
              <option key={c} value={c}>
                {KYC_REASON_LABELS[c]}
              </option>
            ))}
          </select>
        </div>
        {fieldError ? (
          <p className="text-sm text-danger">{fieldError}</p>
        ) : null}
        {error ? <Alert type="error" description={error} /> : null}
      </div>
    </Modal>
  );
}
