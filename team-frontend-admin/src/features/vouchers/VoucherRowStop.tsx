"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import type { VoucherView } from "@/lib/gateway/vouchers";
import { usePending } from "@/lib/usePending";

import { deactivateVoucherAction } from "./actions";

export function VoucherRowStop({ voucher }: { voucher: VoucherView }) {
  const { pending, run } = usePending();
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");

  if (!voucher.active) return null;

  function close() {
    if (pending) return;
    setError("");
    setOpen(false);
  }

  async function onStop() {
    if (pending) return;
    setError("");
    const res = await run(() => deactivateVoucherAction(voucher.id));
    if (res.ok) {
      setOpen(false);
      return;
    }
    setError(res.error);
  }

  return (
    <>
      <Button variant="danger" size="sm" onClick={() => setOpen(true)}>
        Ngừng voucher
      </Button>
      <Modal
        isOpen={open}
        onClose={close}
        title="Ngừng voucher"
        description={`Voucher ${voucher.code} sẽ không còn áp dụng được cho đơn hàng mới. Thao tác này không hoàn tác được.`}
        footer={
          <>
            <Button variant="secondary" onClick={close} disabled={pending}>
              Quay lại
            </Button>
            <Button
              variant="danger"
              onClick={onStop}
              isLoading={pending}
              disabled={pending}
            >
              Xác nhận ngừng
            </Button>
          </>
        }
      >
        {error ? <Alert type="error" description={error} /> : null}
      </Modal>
    </>
  );
}
