"use client";

import { useState } from "react";

import { Button, type ButtonVariant } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { usePendingAction } from "@/features/account/usePendingAction";
import type { ViewAddress } from "@/lib/gateway/addresses";
import { AddressModal } from "./AddressModal";
import { deleteAddressAction, setDefaultAddressAction } from "./actions";

/** "Thêm địa chỉ mới" (header) and "Thêm địa chỉ ngay" (empty state): opens the create Modal. */
export function AddAddressButton({
  label = "Thêm địa chỉ mới",
  variant = "primary",
  className = "",
}: {
  label?: string;
  variant?: ButtonVariant;
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        variant={variant}
        className={`min-h-10 ${className}`}
        onClick={() => setOpen(true)}
      >
        {label}
      </Button>
      {open && <AddressModal onClose={() => setOpen(false)} />}
    </>
  );
}

const compact = "min-h-10";

/**
 * Edit, delete and set-default for one address card. Delete asks for
 * confirmation in a Modal (no window.confirm); every action shows a pending
 * state and ends with a toast. The default address cannot be deleted or
 * re-defaulted, so it only offers "Sửa".
 */
export function AddressActions({ address }: { address: ViewAddress }) {
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const { pending: deleting, run: runDelete } = usePendingAction();
  const { pending: settingDefault, run: runDefault } = usePendingAction();
  const toast = useToast();

  function remove() {
    void runDelete(async () => {
      const res = await deleteAddressAction(address.id);
      if (res.ok) {
        setConfirming(false);
        toast.success("Đã xóa địa chỉ.");
      } else {
        toast.error(res.error);
      }
    });
  }

  function makeDefault() {
    void runDefault(async () => {
      const res = await setDefaultAddressAction(address.id);
      if (res.ok) toast.success("Đã đặt làm địa chỉ mặc định.");
      else toast.error(res.error);
    });
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        size="sm"
        variant="ghost"
        className={compact}
        onClick={() => setEditing(true)}
      >
        Sửa
      </Button>
      {!address.isDefault && (
        <>
          <Button
            size="sm"
            variant="ghost"
            className={`${compact} text-danger`}
            disabled={deleting}
            onClick={() => setConfirming(true)}
          >
            Xóa
          </Button>
          <Button
            size="sm"
            variant="outline"
            className={compact}
            isLoading={settingDefault}
            onClick={makeDefault}
          >
            Thiết lập mặc định
          </Button>
        </>
      )}

      {editing && (
        <AddressModal address={address} onClose={() => setEditing(false)} />
      )}

      <Modal
        isOpen={confirming}
        onClose={deleting ? () => {} : () => setConfirming(false)}
        title="Xóa địa chỉ?"
        description={`Địa chỉ của ${address.recipientName} sẽ bị xóa khỏi sổ địa chỉ.`}
        size="sm"
        footer={
          <>
            <Button
              variant="outline"
              disabled={deleting}
              onClick={() => setConfirming(false)}
            >
              Hủy
            </Button>
            <Button variant="danger" isLoading={deleting} onClick={remove}>
              Xóa địa chỉ
            </Button>
          </>
        }
      >
        <p className="text-sm text-text-secondary">
          Bạn có chắc chắn muốn xóa địa chỉ này? Thao tác không thể hoàn tác.
        </p>
      </Modal>
    </div>
  );
}
