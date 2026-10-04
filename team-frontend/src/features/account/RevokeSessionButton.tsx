"use client";

import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { Tag } from "@/components/ui/Tag";
import { useToast } from "@/components/ui/ToastProvider";
import { revokeSessionAction } from "./actions";
import { usePendingAction } from "./usePendingAction";

/**
 * Revoke one active session. Asks for confirmation in a Modal, shows a pending
 * state on the confirm button and ends with a toast; the Server Action
 * revalidates /account/security, which then renders the row as revoked.
 */
export function RevokeSessionButton({
  sessionId,
  device,
}: {
  sessionId: string;
  device?: string;
}) {
  const [open, setOpen] = useState(false);
  const [revoked, setRevoked] = useState(false);
  const { pending, run } = usePendingAction();
  const toast = useToast();

  function revoke() {
    void run(async () => {
      const res = await revokeSessionAction(sessionId);
      if (res.ok) {
        setRevoked(true);
        setOpen(false);
        toast.success("Đã thu hồi phiên đăng nhập.");
      } else {
        toast.error(res.error);
      }
    });
  }

  if (revoked) return <Tag className="whitespace-nowrap">Đã thu hồi</Tag>;

  return (
    <>
      <Button
        size="sm"
        variant="outline"
        className="min-h-10 text-danger"
        onClick={() => setOpen(true)}
      >
        Thu hồi
      </Button>
      <Modal
        isOpen={open}
        onClose={pending ? () => {} : () => setOpen(false)}
        title="Thu hồi phiên đăng nhập?"
        description={
          device
            ? `Thiết bị ${device} sẽ bị đăng xuất khỏi tài khoản.`
            : "Thiết bị này sẽ bị đăng xuất khỏi tài khoản."
        }
        size="sm"
        footer={
          <>
            <Button
              variant="outline"
              disabled={pending}
              onClick={() => setOpen(false)}
            >
              Hủy
            </Button>
            <Button variant="danger" isLoading={pending} onClick={revoke}>
              Thu hồi phiên
            </Button>
          </>
        }
      >
        <p className="text-sm text-text-secondary">
          Người dùng trên thiết bị này cần đăng nhập lại để tiếp tục.
        </p>
      </Modal>
    </>
  );
}
