"use client";

import { useId, useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Link } from "@/components/ui/Link";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";
import { useToast } from "@/components/ui/ToastProvider";
import { loginPath } from "@/features/auth/returnUrlShared";
import { usePending } from "@/features/seller/usePending";

import { reportListingAction } from "./reportActions";

const LABEL = "Báo cáo tin đăng";

const REASONS = [
  { value: "prohibited_item", label: "Hàng cấm" },
  { value: "counterfeit", label: "Hàng giả" },
  { value: "scam", label: "Lừa đảo" },
  { value: "misleading", label: "Thông tin gây hiểu lầm" },
  { value: "inappropriate", label: "Nội dung không phù hợp" },
  { value: "other", label: "Khác" },
];

const linkClass =
  "text-sm text-text-secondary underline hover:text-text-primary";

/** Buyer "Báo cáo tin đăng" control: login link for a guest, else a dialog. */
export function ReportListingButton({
  listingId,
  loggedIn,
}: { listingId: string; loggedIn: boolean }) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const { pending, run } = usePending();
  const toast = useToast();
  const id = useId();

  if (!loggedIn) {
    return (
      <span className="text-sm text-text-secondary">
        <Link href={loginPath(`/listing/${listingId}`)} variant="subtle">
          {LABEL}
        </Link>
      </span>
    );
  }

  function close() {
    if (pending) return;
    setOpen(false);
    setError("");
  }

  async function onSubmit() {
    if (pending) return;
    if (!reason) {
      setError("Vui lòng chọn lý do.");
      return;
    }
    setError("");
    const res = await run(() => reportListingAction(listingId, reason, note));
    if (res.ok) {
      setOpen(false);
      setReason("");
      setNote("");
      toast.success(res.message);
    } else {
      setError(res.error);
    }
  }

  return (
    <>
      <button type="button" className={linkClass} onClick={() => setOpen(true)}>
        {LABEL}
      </button>
      <Modal
        isOpen={open}
        onClose={close}
        size="sm"
        title={LABEL}
        footer={
          <>
            <Button variant="outline" onClick={close} disabled={pending}>
              Huỷ
            </Button>
            <Button onClick={onSubmit} disabled={pending} isLoading={pending}>
              Gửi báo cáo
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          {error && <Alert type="error" description={error} />}
          <Select
            id={`${id}-reason`}
            label="Lý do"
            placeholder="Chọn lý do"
            options={REASONS}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
          <div className="space-y-1">
            <label htmlFor={`${id}-note`} className="text-sm font-medium">
              Chi tiết (không bắt buộc)
            </label>
            <textarea
              id={`${id}-note`}
              maxLength={500}
              value={note}
              onChange={(e) => setNote(e.target.value)}
              className="block w-full rounded-lg border border-border-subtle p-2 text-sm"
              rows={3}
            />
            <p className="text-right text-xs text-text-muted">
              {note.length}/500
            </p>
          </div>
        </div>
      </Modal>
    </>
  );
}
