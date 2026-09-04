"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { requestWalletPayoutAction } from "./actions";

/** Request a payout of the full wallet balance (mock). Thin. */
export function WalletPayoutButton({
  sellerId,
  balance,
}: {
  sellerId: string;
  balance: number;
}) {
  const [pending, start] = useTransition();
  const [done, setDone] = useState(false);
  const toast = useToast();

  function payout() {
    start(async () => {
      const res = await requestWalletPayoutAction(sellerId, balance);
      if (res.ok) {
        setDone(true);
        toast.success("✓ Đã tạo lệnh rút tiền.");
      } else {
        toast.error(res.message || "Rút tiền thất bại.");
      }
    });
  }

  return (
    <button
      type="button"
      onClick={payout}
      disabled={pending || balance <= 0}
      className="rounded-xl bg-emerald-600 px-4 py-2.5 text-sm font-bold text-white shadow-sm transition hover:bg-emerald-700 disabled:opacity-50"
    >
      {pending
        ? "⏳ Đang xử lý..."
        : done
          ? "✓ Đã gửi yêu cầu"
          : "💳 Rút toàn bộ số dư"}
    </button>
  );
}
