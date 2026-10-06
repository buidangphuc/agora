"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button, type ButtonVariant } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { updateOrderStatusAction } from "@/features/order/actions";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { usePending } from "./usePending";

/**
 * "Xác nhận gửi" / "Bàn giao vận chuyển": moves one order to SHIPPED through
 * updateOrderStatusAction. It is pending and disabled on its own row only.
 * With `withConfirm` (order detail) a confirm Modal asks first and takes an
 * optional carrier tracking number; no tracking number is ever invented.
 */
export function ShipOrderButton({
  orderId,
  label = "Xác nhận gửi",
  variant = "outline",
  size = "sm",
  withConfirm = false,
}: {
  orderId: string;
  label?: string;
  variant?: ButtonVariant;
  size?: "xs" | "sm" | "md";
  withConfirm?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [tracking, setTracking] = useState("");
  const [error, setError] = useState("");
  const { pending, run } = usePending();
  const toast = useToast();

  async function ship() {
    setError("");
    const res = await run(() =>
      updateOrderStatusAction(
        orderId,
        OrderStatus.SHIPPED,
        tracking.trim() || undefined,
      ),
    );
    if (res.ok) {
      setOpen(false);
      toast.success("Đã bàn giao vận chuyển");
    } else {
      setError(res.error);
      toast.error(res.error);
    }
  }

  function close() {
    if (pending) return;
    setOpen(false);
    setError("");
  }

  return (
    <>
      <Button
        size={size}
        variant={variant}
        isLoading={pending && !withConfirm}
        onClick={() => (withConfirm ? setOpen(true) : ship())}
      >
        {label}
      </Button>
      {withConfirm && (
        <Modal
          isOpen={open}
          onClose={close}
          size="sm"
          title="Bàn giao cho đơn vị vận chuyển?"
          description="Đơn hàng sẽ chuyển sang trạng thái Đang giao."
          footer={
            <>
              <Button variant="outline" onClick={close} disabled={pending}>
                Huỷ
              </Button>
              <Button onClick={ship} isLoading={pending}>
                Xác nhận bàn giao
              </Button>
            </>
          }
        >
          <div className="space-y-3">
            <Input
              label="Mã vận đơn (nếu có)"
              value={tracking}
              onChange={(e) => setTracking(e.target.value)}
              disabled={pending}
            />
            {error && <Alert type="error" description={error} />}
          </div>
        </Modal>
      )}
    </>
  );
}
