"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Link } from "@/components/ui/Link";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { formatPrice } from "@/components/ui/format";
import { subscribeAction } from "./actions";
import { formatPeriodEnd } from "./periodEnd";
import { usePending } from "./usePending";

export type PlanDirection = "upgrade" | "downgrade" | "current";

const INSUFFICIENT = "Số dư có thể rút không đủ để trả gói này.";

export function SubscribeButton({
  planId,
  price,
  termDays,
  direction,
  periodEnd,
}: {
  planId: string;
  price: number;
  termDays: number;
  direction: PlanDirection;
  periodEnd?: string;
}) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const { pending, run } = usePending();
  const toast = useToast();

  if (direction === "current") {
    return (
      <Button variant="outline" fullWidth disabled aria-disabled="true">
        Gói hiện tại
      </Button>
    );
  }

  const upgrade = direction === "upgrade";
  const label = upgrade ? "Nâng cấp" : "Hạ gói";

  function close() {
    if (pending) return;
    setOpen(false);
    setError("");
  }

  async function confirm() {
    setError("");
    const res = await run(() => subscribeAction(planId));
    if (res.ok) {
      setOpen(false);
      toast.success("Đã đăng ký gói");
    } else {
      setError(res.error);
      toast.error(res.error);
    }
  }

  return (
    <>
      <Button variant="outline" fullWidth onClick={() => setOpen(true)}>
        {label}
      </Button>
      <Modal
        isOpen={open}
        onClose={close}
        size="sm"
        title={`${label} gói`}
        footer={
          <>
            <Button variant="outline" onClick={close} disabled={pending}>
              Huỷ
            </Button>
            <Button onClick={confirm} isLoading={pending} disabled={pending}>
              Xác nhận đăng ký
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          <p className="text-sm text-text-primary">
            Giá gói: {formatPrice(price)}, trừ từ số dư có thể rút của bạn.
          </p>
          <p className="text-sm text-text-primary">
            {upgrade
              ? `Nâng cấp có hiệu lực ngay, tính đủ giá gói mới, không hoàn tiền phần còn lại, chu kỳ ${termDays} ngày mới.`
              : `Hạ gói có hiệu lực từ ngày ${formatPeriodEnd(periodEnd ?? "")} (hết chu kỳ hiện tại).`}
          </p>
          {error && <Alert type="error" description={error} />}
          {error === INSUFFICIENT && <Link href="/seller/wallet">Xem ví</Link>}
        </div>
      </Modal>
    </>
  );
}
