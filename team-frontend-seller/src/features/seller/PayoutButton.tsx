"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { formatPrice } from "@/components/ui/format";
import { requestWalletPayoutAction } from "./actions";
import { usePending } from "./usePending";

/** Error text for a payout amount, or null when it is a valid whole amount <= balance. */
export function validatePayoutAmount(
  value: string,
  balance: number,
): string | null {
  const amount = Number(value);
  if (value.trim() === "" || !Number.isFinite(amount) || amount <= 0) {
    return "Nhập số tiền lớn hơn 0.";
  }
  if (!Number.isInteger(amount)) return "Số tiền phải là số nguyên.";
  if (amount > balance) return "Số tiền vượt quá số dư khả dụng.";
  return null;
}

/**
 * "Rút tiền": opens a confirm Modal (amount defaults to the whole balance).
 * Disabled with an explanation while the balance is 0. The Server Action
 * revalidates /seller/wallet, so balance and ledger refresh on success.
 */
export function PayoutButton({
  sellerId,
  balance,
}: { sellerId: string; balance: number }) {
  const [open, setOpen] = useState(false);
  const [amount, setAmount] = useState(String(balance));
  const [error, setError] = useState("");
  const { pending, run } = usePending();
  const toast = useToast();
  const empty = balance <= 0;
  const fieldError = validatePayoutAmount(amount, balance);

  function show() {
    setAmount(String(balance));
    setError("");
    setOpen(true);
  }

  function close() {
    if (pending) return;
    setOpen(false);
  }

  async function confirmPayout() {
    if (fieldError) return;
    setError("");
    const res = await run(() =>
      requestWalletPayoutAction(sellerId, Number(amount)),
    );
    if (res.ok) {
      setOpen(false);
      toast.success("Đã tạo lệnh rút tiền");
    } else {
      setError(res.error);
      toast.error(res.error);
    }
  }

  return (
    <>
      <div className="flex flex-col items-start gap-1.5 sm:items-end">
        <Button variant="primary" disabled={empty} onClick={show}>
          Rút tiền
        </Button>
        {empty && (
          <p className="text-xs text-text-secondary">
            Số dư bằng 0, chưa thể rút tiền.
          </p>
        )}
      </div>
      <Modal
        isOpen={open}
        onClose={close}
        size="sm"
        title="Xác nhận rút tiền"
        description={`Số dư khả dụng: ${formatPrice(balance)}`}
        footer={
          <>
            <Button variant="outline" onClick={close} disabled={pending}>
              Huỷ
            </Button>
            <Button
              onClick={confirmPayout}
              isLoading={pending}
              disabled={fieldError !== null}
            >
              Xác nhận rút tiền
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          <Input
            label="Số tiền (₫)"
            type="number"
            min={1}
            max={balance}
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            error={fieldError ?? undefined}
            disabled={pending}
          />
          {error && <Alert type="error" description={error} />}
        </div>
      </Modal>
    </>
  );
}
