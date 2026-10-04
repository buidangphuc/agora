"use client";

import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/ToastProvider";
import { subscribeAction } from "./actions";
import { usePending } from "./usePending";

/**
 * Subscribe the seller to a plan. The current plan is disabled; the Server
 * Action revalidates /seller/plans so the "Gói hiện tại" tag moves.
 */
export function SubscribeButton({
  planId,
  current,
}: {
  planId: string;
  current: boolean;
}) {
  const { pending, run } = usePending();
  const toast = useToast();

  async function subscribe() {
    const res = await run(() => subscribeAction(planId));
    if (res.ok) toast.success("Đã đăng ký gói");
    else toast.error(res.error);
  }

  return (
    <Button
      variant="outline"
      className="w-full"
      disabled={current}
      isLoading={pending}
      onClick={subscribe}
    >
      {current ? "Gói hiện tại" : "Đăng ký"}
    </Button>
  );
}
