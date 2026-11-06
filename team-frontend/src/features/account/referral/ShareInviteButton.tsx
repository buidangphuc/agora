"use client";

import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/ToastProvider";
import { usePendingAction } from "@/features/account/usePendingAction";
import { createReferralInviteLinkAction } from "./actions";

const LINK_ERROR = "Không tạo được link mời. Vui lòng thử lại.";

export function ShareInviteButton({ code }: { code: string }) {
  const toast = useToast();
  const { pending, run } = usePendingAction();

  function share() {
    void run(async () => {
      try {
        const res = await createReferralInviteLinkAction();
        if (!res.ok || !res.data) {
          toast.error(res.ok ? LINK_ERROR : res.error);
          return;
        }
        await navigator.clipboard.writeText(
          `${window.location.origin}/s/${res.data.shortCode}`,
        );
        toast.success("Đã sao chép link mời.");
      } catch {
        toast.error(LINK_ERROR);
      }
    });
  }

  return (
    <Button size="sm" isLoading={pending} disabled={!code} onClick={share}>
      Chia sẻ link mời
    </Button>
  );
}
