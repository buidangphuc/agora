"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import type { VoucherView } from "@/lib/gateway/vouchers";
import { usePending } from "@/lib/usePending";

import { deactivateVoucherAction, updateVoucherAction } from "./actions";
import {
  type VoucherEditErrors,
  type VoucherEditValues,
  validateEditForm,
} from "./voucherForm";
import { toVnDateTimeLocal } from "./voucherFormat";

export function VoucherActions({ voucher }: { voucher: VoucherView }) {
  const router = useRouter();
  const { pending, run } = usePending();
  const [stopOpen, setStopOpen] = useState(false);
  const [stopError, setStopError] = useState("");
  const [editing, setEditing] = useState(false);
  const [values, setValues] = useState<VoucherEditValues>({
    endsAt: toVnDateTimeLocal(voucher.endsAt),
    quota: String(voucher.quota),
  });
  const [errors, setErrors] = useState<VoucherEditErrors>({});
  const [editError, setEditError] = useState("");

  if (!voucher.active) return null;

  function closeStop() {
    if (pending) return;
    setStopError("");
    setStopOpen(false);
  }

  async function onStop() {
    if (pending) return;
    setStopError("");
    const res = await run(() => deactivateVoucherAction(voucher.id));
    if (res.ok) {
      setStopOpen(false);
      router.refresh();
      return;
    }
    setStopError(res.error);
  }

  async function onSave() {
    if (pending) return;
    setEditError("");
    const checked = validateEditForm(values, voucher, new Date());
    if (!checked.ok) {
      setErrors(checked.errors);
      return;
    }
    setErrors({});
    const res = await run(() => updateVoucherAction(voucher, values));
    if (res.ok) {
      setEditing(false);
      router.refresh();
      return;
    }
    if (res.fieldErrors) {
      setErrors(res.fieldErrors);
      return;
    }
    setEditError(res.error);
  }

  return (
    <div className="flex flex-col gap-4" data-testid="voucher-actions">
      <div className="flex flex-col gap-3 sm:flex-row">
        <Button
          variant="secondary"
          fullWidth="mobile"
          onClick={() => setEditing(true)}
          disabled={editing}
        >
          Sửa voucher
        </Button>
        <Button
          variant="danger"
          fullWidth="mobile"
          onClick={() => setStopOpen(true)}
        >
          Ngừng voucher
        </Button>
      </div>

      {editing ? (
        <form
          noValidate
          data-testid="voucher-edit-form"
          onSubmit={(e) => {
            e.preventDefault();
            void onSave();
          }}
          className="grid grid-cols-1 gap-5 md:grid-cols-2"
        >
          <Input
            label="Kết thúc (giờ Việt Nam)"
            name="endsAt"
            type="datetime-local"
            required
            value={values.endsAt}
            onChange={(e) =>
              setValues((v) => ({ ...v, endsAt: e.target.value }))
            }
            error={errors.endsAt}
          />
          {voucher.quota > 0 ? (
            <Input
              label="Lượt dùng tối đa"
              name="quota"
              inputMode="numeric"
              value={values.quota}
              onChange={(e) =>
                setValues((v) => ({ ...v, quota: e.target.value }))
              }
              helperText="Chỉ có thể tăng, không thể giảm hay bỏ giới hạn."
              error={errors.quota}
            />
          ) : null}
          {editError ? (
            <div data-testid="voucher-edit-error" className="md:col-span-2">
              <Alert type="error" description={editError} />
            </div>
          ) : null}
          <div className="flex flex-col gap-3 sm:flex-row md:col-span-2">
            <Button type="submit" isLoading={pending} disabled={pending}>
              Lưu thay đổi
            </Button>
            <Button
              variant="secondary"
              onClick={() => {
                setEditing(false);
                setErrors({});
                setEditError("");
              }}
              disabled={pending}
            >
              Huỷ
            </Button>
          </div>
        </form>
      ) : null}

      <Modal
        isOpen={stopOpen}
        onClose={closeStop}
        title="Ngừng voucher"
        description={`Voucher ${voucher.code} sẽ không còn áp dụng được cho đơn hàng mới. Thao tác này không hoàn tác được.`}
        footer={
          <>
            <Button variant="secondary" onClick={closeStop} disabled={pending}>
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
        {stopError ? <Alert type="error" description={stopError} /> : null}
      </Modal>
    </div>
  );
}
