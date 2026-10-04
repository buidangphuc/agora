"use client";

import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/ToastProvider";

/** Copy the referral code to the clipboard. */
export function CopyCodeButton({ code }: { code: string }) {
  const toast = useToast();

  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      toast.success("Đã sao chép mã giới thiệu.");
    } catch {
      toast.error("Không thể sao chép mã. Hãy chọn và sao chép thủ công.");
    }
  }

  return (
    <Button size="sm" variant="outline" className="min-h-10" onClick={copy}>
      Sao chép
    </Button>
  );
}
