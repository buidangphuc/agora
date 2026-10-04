"use client";

import { type FormEvent, useId, useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { FormItem } from "@/components/ui/FormItem";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { usePendingAction } from "@/features/account/usePendingAction";
import type { ViewAddress } from "@/lib/gateway/addresses";
import { createAddressAction, updateAddressAction } from "./actions";
import {
  type AddressErrors,
  firstInvalid,
  validateAddress,
} from "./validation";

/**
 * Create / edit an address in a Modal. The parent renders it only while open,
 * so it is always `isOpen`; `onClose` is called on cancel, Escape, backdrop and
 * success. Used by the address book and by checkout.
 */
export function AddressModal({
  address,
  onClose,
}: {
  address?: ViewAddress | null;
  onClose: () => void;
}) {
  const isEditing = !!address;
  const formId = useId();
  const [errors, setErrors] = useState<AddressErrors>({});
  const [formError, setFormError] = useState("");
  const { pending, run } = usePendingAction();
  const toast = useToast();

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;
    const form = event.currentTarget;
    const formData = new FormData(form);

    const found = validateAddress(formData);
    setErrors(found);
    const invalid = firstInvalid(found);
    if (invalid) {
      form.querySelector<HTMLInputElement>(`[name="${invalid}"]`)?.focus();
      return;
    }

    setFormError("");
    void run(async () => {
      const res = address
        ? await updateAddressAction(address.id, formData)
        : await createAddressAction(formData);
      if (res.ok) {
        toast.success(
          isEditing ? "Đã cập nhật địa chỉ." : "Đã thêm địa chỉ thành công.",
        );
        onClose();
      } else {
        setFormError(res.error);
        toast.error(res.error);
      }
    });
  }

  return (
    <Modal
      isOpen
      onClose={pending ? () => {} : onClose}
      title={isEditing ? "Chỉnh sửa địa chỉ" : "Thêm địa chỉ mới"}
      size="lg"
      footer={
        <>
          <Button variant="outline" disabled={pending} onClick={onClose}>
            Hủy
          </Button>
          <Button type="submit" form={formId} isLoading={pending}>
            {isEditing ? "Cập nhật" : "Thêm mới"}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={submit} noValidate className="space-y-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <FormItem
            label="Họ và tên"
            required
            help={errors.recipientName}
            status={errors.recipientName ? "error" : undefined}
          >
            <Input
              name="recipientName"
              required
              defaultValue={address?.recipientName}
              placeholder="VD: Nguyễn Văn A"
              autoComplete="name"
            />
          </FormItem>
          <FormItem
            label="Số điện thoại"
            required
            help={errors.phone}
            status={errors.phone ? "error" : undefined}
          >
            <Input
              name="phone"
              type="tel"
              required
              defaultValue={address?.phone}
              placeholder="VD: 0912345678"
              autoComplete="tel"
            />
          </FormItem>
        </div>

        <FormItem
          label="Địa chỉ chi tiết (số nhà, tên đường)"
          required
          help={errors.street}
          status={errors.street ? "error" : undefined}
        >
          <Input
            name="street"
            required
            defaultValue={address?.street}
            placeholder="VD: 123 Đường Nguyễn Huệ"
            autoComplete="street-address"
          />
        </FormItem>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <FormItem label="Phường / Xã">
            <Input
              name="ward"
              defaultValue={address?.ward}
              placeholder="VD: P. Bến Nghé"
            />
          </FormItem>
          <FormItem label="Quận / Huyện">
            <Input
              name="district"
              defaultValue={address?.district}
              placeholder="VD: Quận 1"
            />
          </FormItem>
          <FormItem
            label="Tỉnh / Thành phố"
            required
            help={errors.city}
            status={errors.city ? "error" : undefined}
          >
            <Input
              name="city"
              required
              defaultValue={address?.city}
              placeholder="VD: TP. HCM"
              autoComplete="address-level1"
            />
          </FormItem>
        </div>

        <Checkbox
          name="isDefault"
          label="Đặt làm địa chỉ mặc định"
          defaultChecked={address?.isDefault}
        />

        {formError && <Alert type="error" description={formError} />}
      </form>
    </Modal>
  );
}
