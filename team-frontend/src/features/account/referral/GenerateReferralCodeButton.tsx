"use client";

import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/ToastProvider";
import { usePendingAction } from "@/features/account/usePendingAction";
import { ensureReferralCodeAction } from "./actions";

/**
 * Mint the caller's referral code on demand. Pending state on the button, a
 * toast at the end; the action revalidates /account/referral, which then shows
 * the code.
 */
export function GenerateReferralCodeButton() {
  const { pending, run } = usePendingAction();
  const toast = useToast();

  function generate() {
    void run(async () => {
      const res = await ensureReferralCodeAction();
      if (res.ok) toast.success("Đã tạo mã giới thiệu của bạn.");
      else toast.error(res.error);
    });
  }

  return (
    <Button isLoading={pending} onClick={generate} className="min-h-10">
      Tạo mã giới thiệu
    </Button>
  );
}
