"use client";

import { useRouter } from "next/navigation";
import React, { useState } from "react";

import { Button, type ButtonVariant } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";
import { useToast } from "@/components/ui/ToastProvider";
import { ReviewModal } from "@/features/review/ReviewModal";
import { ReturnRequestModal } from "./ReturnRequestModal";
import { useReturnState } from "./ReturnState";
import { cancelOrderAction, reorderAction } from "./actions";
import { usePendingAction } from "./usePendingAction";

export const CANCEL_REASONS = [
  { value: "changed_mind", label: "Tôi muốn thay đổi địa chỉ hoặc đổi ý" },
  { value: "wrong_item", label: "Shop tư vấn hoặc giao sai phân loại" },
  { value: "damaged", label: "Sản phẩm bị lỗi / hư hỏng" },
  { value: "delivery_delay", label: "Thời gian giao hàng quá lâu" },
];

const fullOnMobile = "w-full sm:w-auto";

/** Re-adds the order's items to the cart, then goes to /cart. */
function useReorder(orderId: string) {
  const toast = useToast();
  const router = useRouter();
  const [pending, run] = usePendingAction();
  function reorder() {
    if (pending) return;
    void run(async () => {
      const res = await reorderAction(orderId);
      if (res.ok) {
        toast.success("Đã thêm lại sản phẩm vào giỏ hàng.");
        router.push("/cart");
      } else {
        toast.error(res.error);
      }
    });
  }
  return { pending, reorder };
}

/** "Mua lại" on its own (used by the timeline failure alert). */
export function ReorderButton({
  orderId,
  variant = "primary",
  size = "sm",
}: {
  orderId: string;
  variant?: ButtonVariant;
  size?: "xs" | "sm" | "md";
}) {
  const { pending, reorder } = useReorder(orderId);
  return (
    <Button variant={variant} size={size} isLoading={pending} onClick={reorder}>
      Mua lại
    </Button>
  );
}

/** "Đánh giá" for one item of a completed order; opens the existing ReviewModal. */
export function ReviewButton({
  listingId,
  orderId,
  productTitle,
}: {
  listingId: string;
  orderId: string;
  productTitle: string;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="outline" size="xs" onClick={() => setOpen(true)}>
        Đánh giá
      </Button>
      {open && (
        <ReviewModal
          listingId={listingId}
          orderId={orderId}
          productTitle={productTitle}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}

export interface OrderActionsProps {
  orderId: string;
  orderTotal: number;
  /** Detail header: "Mua lại" is the primary action; list rows use outline. */
  primaryReorder?: boolean;
  canCancel: boolean;
  canReturn: boolean;
}

/**
 * Reorder, cancel (confirmation Modal) and return (request Modal) triggers.
 * Renders a fragment of buttons so the parent decides the layout. While one
 * action is pending the others are disabled and the Modals cannot be dismissed.
 */
export function OrderActions({
  orderId,
  orderTotal,
  primaryReorder = false,
  canCancel,
  canReturn,
}: OrderActionsProps) {
  const toast = useToast();
  const reorder = useReorder(orderId);
  const [cancelOpen, setCancelOpen] = useState(false);
  const [reason, setReason] = useState(CANCEL_REASONS[0].value);
  const [cancelling, runCancel] = usePendingAction();
  const [returnOpen, setReturnOpen] = useState(false);
  const [ret, setRet] = useReturnState(null);

  const busy = reorder.pending || cancelling;

  function closeCancel() {
    if (!cancelling) setCancelOpen(false);
  }

  function confirmCancel() {
    if (cancelling) return;
    const label = CANCEL_REASONS.find((r) => r.value === reason)?.label;
    void runCancel(async () => {
      const res = await cancelOrderAction(orderId, label ?? reason);
      if (res.ok) {
        toast.success("Đã hủy đơn hàng thành công.");
        setCancelOpen(false);
      } else {
        toast.error(res.error);
      }
    });
  }

  return (
    <>
      <Button
        variant={primaryReorder ? "primary" : "outline"}
        size={primaryReorder ? "md" : "sm"}
        className={fullOnMobile}
        isLoading={reorder.pending}
        disabled={cancelling}
        onClick={reorder.reorder}
      >
        Mua lại
      </Button>

      {canReturn && !ret && (
        <Button
          variant="outline"
          size={primaryReorder ? "md" : "sm"}
          className={fullOnMobile}
          disabled={busy}
          onClick={() => setReturnOpen(true)}
        >
          Yêu cầu trả hàng
        </Button>
      )}

      {canCancel && (
        <Button
          variant="outline"
          size={primaryReorder ? "md" : "sm"}
          className={fullOnMobile}
          disabled={busy}
          onClick={() => setCancelOpen(true)}
        >
          Hủy đơn
        </Button>
      )}

      <Modal
        isOpen={cancelOpen}
        onClose={closeCancel}
        size="sm"
        title="Hủy đơn hàng"
        description="Hệ thống sẽ hoàn lại tồn kho cho người bán. Vui lòng chọn lý do hủy."
        footer={
          <>
            <Button
              variant="outline"
              disabled={cancelling}
              onClick={closeCancel}
            >
              Không hủy
            </Button>
            <Button
              variant="danger"
              data-testid="cancel-confirm"
              isLoading={cancelling}
              onClick={confirmCancel}
            >
              Xác nhận hủy đơn
            </Button>
          </>
        }
      >
        <Select
          aria-label="Lý do hủy đơn"
          value={reason}
          disabled={cancelling}
          options={CANCEL_REASONS}
          onChange={(e) => setReason(e.target.value)}
        />
      </Modal>

      <ReturnRequestModal
        isOpen={returnOpen}
        onClose={() => setReturnOpen(false)}
        orderId={orderId}
        orderTotal={orderTotal}
        onCreated={setRet}
      />
    </>
  );
}
