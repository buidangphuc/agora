"use client";

import React, { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { confirmReceiptAction } from "./actions";
import { usePendingAction } from "./usePendingAction";

/** "Đã nhận được hàng" on a shipped order; confirms in a dialog first. */
export function ConfirmReceiptButton({
  orderId,
  size = "sm",
}: {
  orderId: string;
  size?: "xs" | "sm" | "md";
}) {
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [pending, run] = usePendingAction();

  function close() {
    if (!pending) setOpen(false);
  }

  function confirm() {
    if (pending) return;
    void run(async () => {
      const res = await confirmReceiptAction(orderId);
      if (res.ok) {
        toast.success("Đã xác nhận nhận hàng.");
        setOpen(false);
      } else {
        toast.error(res.error);
      }
    });
  }

  return (
    <>
      <Button
        variant="outline"
        size={size}
        fullWidth="mobile"
        onClick={() => setOpen(true)}
      >
        Đã nhận được hàng
      </Button>
      <Modal
        isOpen={open}
        onClose={close}
        size="sm"
        title="Xác nhận nhận hàng"
        description="Xác nhận bạn đã nhận được hàng? Đơn hàng sẽ được hoàn thành."
        footer={
          <>
            <Button variant="outline" disabled={pending} onClick={close}>
              Chưa
            </Button>
            <Button
              data-testid="receipt-confirm"
              isLoading={pending}
              onClick={confirm}
            >
              Xác nhận
            </Button>
          </>
        }
      >
        {null}
      </Modal>
    </>
  );
}
