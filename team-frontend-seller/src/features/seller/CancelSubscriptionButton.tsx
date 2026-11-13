"use client";

import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { formatPeriodEnd } from "./SubscribeButton";
import { cancelSubscriptionAction } from "./actions";
import { usePending } from "./usePending";

export function CancelSubscriptionButton({
  paid,
  autoRenew,
  periodEnd,
}: {
  paid: boolean;
  autoRenew: boolean;
  periodEnd?: string;
}) {
  const [open, setOpen] = useState(false);
  const { pending, run } = usePending();
  const toast = useToast();

  if (!paid) return null;
  const end = formatPeriodEnd(periodEnd ?? "");
  if (!autoRenew) {
    return (
      <p className="text-sm text-text-secondary">
        Sẽ chuyển về Miễn phí ngày {end}
      </p>
    );
  }

  function close() {
    if (pending) return;
    setOpen(false);
  }

  async function confirm() {
    const res = await run(() => cancelSubscriptionAction());
    setOpen(false);
    if (res.ok) toast.success("Đã hủy gia hạn");
    else toast.error(res.error);
  }

  return (
    <>
      <Button variant="outline" fullWidth onClick={() => setOpen(true)}>
        Hủy gia hạn
      </Button>
      <Modal
        isOpen={open}
        onClose={close}
        size="sm"
        title="Hủy gia hạn?"
        footer={
          <>
            <Button variant="outline" onClick={close} disabled={pending}>
              Huỷ
            </Button>
            <Button
              variant="danger"
              onClick={confirm}
              isLoading={pending}
              disabled={pending}
            >
              Xác nhận hủy gia hạn
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          <p className="text-sm text-text-primary">
            Gói vẫn dùng được đến ngày {end}, sau đó chuyển về Miễn phí.
          </p>
        </div>
      </Modal>
    </>
  );
}
