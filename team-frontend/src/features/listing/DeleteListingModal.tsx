"use client";

import { useCallback, useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { usePending } from "@/features/seller/usePending";
import { deleteListingAction } from "./actions";

/**
 * "Xoá" row action: opens a confirm Modal naming the listing. While the Server
 * Action is pending the confirm button spins and the dismiss controls are
 * disabled; on failure the Modal stays open with an Alert and a toast. The
 * action revalidates /seller, so the row disappears on success.
 */
export function DeleteListingModal({
  id,
  title,
}: { id: string; title: string }) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const { pending, run } = usePending();
  const toast = useToast();
  const focusCancel = useCallback((el: HTMLButtonElement | null) => {
    el?.focus();
  }, []);

  function close() {
    if (pending) return;
    setOpen(false);
    setError("");
  }

  async function confirm() {
    setError("");
    const res = await run(() => deleteListingAction(id));
    if (res.ok) {
      setOpen(false);
      toast.success("Đã xoá sản phẩm");
    } else {
      setError(res.error);
      toast.error(res.error);
    }
  }

  return (
    <>
      <Button
        size="xs"
        variant="ghost"
        className="text-danger"
        onClick={() => setOpen(true)}
        aria-label={`Xoá ${title}`}
      >
        Xoá
      </Button>
      <Modal
        isOpen={open}
        onClose={close}
        size="sm"
        title={`Xoá "${title}"?`}
        description="Hành động này không thể hoàn tác."
        footer={
          <>
            {/* The dialog opens focused on the safe choice (Cancel). */}
            <Button
              variant="outline"
              onClick={close}
              disabled={pending}
              ref={focusCancel}
            >
              Huỷ
            </Button>
            <Button variant="danger" onClick={confirm} isLoading={pending}>
              Xoá sản phẩm
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          <p className="text-sm text-text-primary">
            Sản phẩm sẽ bị gỡ khỏi gian hàng và không còn hiển thị cho người
            mua.
          </p>
          {error && <Alert type="error" description={error} />}
        </div>
      </Modal>
    </>
  );
}
