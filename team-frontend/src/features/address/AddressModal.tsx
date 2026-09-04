"use client";

import { useEffect } from "react";
import { useFormState, useFormStatus } from "react-dom";

import type { ViewAddress } from "@/lib/gateway/addresses";
import {
  type AddressState,
  createAddressAction,
  updateAddressAction,
} from "./actions";

const initialState: AddressState = { ok: false, message: "" };

const labelClass =
  "block text-xs font-semibold uppercase tracking-wider text-gray-600";
const fieldClass =
  "mt-1 w-full rounded-md border border-gray-300 px-3 py-2 text-sm outline-none focus:border-brand";

function SubmitButton({ isEditing }: { isEditing: boolean }) {
  const { pending } = useFormStatus();
  return (
    <button
      type="submit"
      disabled={pending}
      className="rounded-md bg-brand px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-dark disabled:opacity-60"
    >
      {pending ? "Đang lưu..." : isEditing ? "Cập nhật" : "Thêm mới"}
    </button>
  );
}

export function AddressModal({
  address,
  onClose,
}: {
  address?: ViewAddress | null;
  onClose: () => void;
}) {
  const isEditing = !!address;
  const action = isEditing
    ? updateAddressAction.bind(null, address.id)
    : createAddressAction;
  const [state, formAction] = useFormState(action, initialState);

  useEffect(() => {
    if (state.ok) {
      onClose();
    }
  }, [state.ok, onClose]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4 backdrop-blur-xs">
      <div className="w-full max-w-lg rounded-xl bg-white p-6 shadow-xl animate-in fade-in zoom-in-95">
        <div className="mb-4 flex items-center justify-between border-b pb-3">
          <h2 className="text-lg font-semibold text-gray-900">
            {isEditing ? "Chỉnh sửa địa chỉ" : "Thêm địa chỉ mới"}
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600"
          >
            ✕
          </button>
        </div>

        <form action={formAction} className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className={labelClass} htmlFor="recipientName">
                Họ và tên *
              </label>
              <input
                id="recipientName"
                name="recipientName"
                required
                defaultValue={address?.recipientName}
                placeholder="VD: Nguyễn Văn A"
                className={fieldClass}
              />
            </div>
            <div>
              <label className={labelClass} htmlFor="phone">
                Số điện thoại *
              </label>
              <input
                id="phone"
                name="phone"
                required
                defaultValue={address?.phone}
                placeholder="VD: 0912345678"
                className={fieldClass}
              />
            </div>
          </div>

          <div>
            <label className={labelClass} htmlFor="street">
              Địa chỉ chi tiết (Số nhà, tên đường) *
            </label>
            <input
              id="street"
              name="street"
              required
              defaultValue={address?.street}
              placeholder="VD: 123 Đường Nguyễn Huệ"
              className={fieldClass}
            />
          </div>

          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className={labelClass} htmlFor="ward">
                Phường / Xã
              </label>
              <input
                id="ward"
                name="ward"
                defaultValue={address?.ward}
                placeholder="VD: P. Bến Nghé"
                className={fieldClass}
              />
            </div>
            <div>
              <label className={labelClass} htmlFor="district">
                Quận / Huyện
              </label>
              <input
                id="district"
                name="district"
                defaultValue={address?.district}
                placeholder="VD: Quận 1"
                className={fieldClass}
              />
            </div>
            <div>
              <label className={labelClass} htmlFor="city">
                Tỉnh / Thành phố *
              </label>
              <input
                id="city"
                name="city"
                required
                defaultValue={address?.city}
                placeholder="VD: TP. HCM"
                className={fieldClass}
              />
            </div>
          </div>

          <div className="flex items-center gap-2 pt-2">
            <input
              id="isDefault"
              name="isDefault"
              type="checkbox"
              defaultChecked={address?.isDefault}
              className="h-4 w-4 rounded border-gray-300 text-brand focus:ring-brand"
            />
            <label htmlFor="isDefault" className="text-sm text-gray-700">
              Đặt làm địa chỉ mặc định
            </label>
          </div>

          {state.message && !state.ok && (
            <p className="rounded bg-red-50 p-2 text-xs text-red-600">
              {state.message}
            </p>
          )}

          <div className="flex justify-end gap-3 border-t pt-4">
            <button
              type="button"
              onClick={onClose}
              className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
            >
              Hủy
            </button>
            <SubmitButton isEditing={isEditing} />
          </div>
        </form>
      </div>
    </div>
  );
}
