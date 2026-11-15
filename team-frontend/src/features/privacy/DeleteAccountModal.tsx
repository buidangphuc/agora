"use client";

import { useEffect, useState } from "react";

import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { usePendingAction } from "../account/usePendingAction";
import { requestDeletionAction } from "./actions";

export function DeleteAccountModal({
  isOpen,
  onClose,
  onScheduled,
}: {
  isOpen: boolean;
  onClose: () => void;
  onScheduled: () => void;
}) {
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const { pending, run } = usePendingAction();

  useEffect(() => {
    if (!isOpen) {
      setPassword("");
      setError("");
    }
  }, [isOpen]);

  function submit() {
    if (password === "") return;
    void run(async () => {
      setError("");
      const res = await requestDeletionAction(password);
      setPassword("");
      if (res.ok) {
        onScheduled();
      } else {
        setError(res.error);
      }
    });
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={pending ? () => {} : onClose}
      title="Xóa tài khoản?"
      description="Nhập mật khẩu để xác nhận. Tài khoản sẽ bị xóa sau thời gian chờ và bạn có thể hủy trước đó."
      size="sm"
      footer={
        <>
          <Button variant="outline" disabled={pending} onClick={onClose}>
            Đóng
          </Button>
          <Button
            variant="danger"
            isLoading={pending}
            disabled={password === ""}
            onClick={submit}
          >
            Xác nhận xóa
          </Button>
        </>
      }
    >
      <Input
        label="Mật khẩu"
        type="password"
        autoComplete="current-password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        error={error || undefined}
      />
    </Modal>
  );
}
