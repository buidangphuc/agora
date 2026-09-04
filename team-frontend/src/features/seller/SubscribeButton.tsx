"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { subscribeAction } from "./actions";

/** Subscribe the current seller to a plan (mock). Thin. */
export function SubscribeButton({
  planId,
  current,
}: {
  planId: string;
  current: boolean;
}) {
  const [pending, start] = useTransition();
  const [subscribed, setSubscribed] = useState(current);
  const toast = useToast();

  function go() {
    start(async () => {
      const res = await subscribeAction(planId);
      if (res.ok) {
        setSubscribed(true);
        toast.success("✓ Đã đăng ký gói.");
      } else {
        toast.error(res.message || "Đăng ký gói thất bại.");
      }
    });
  }

  return (
    <button
      type="button"
      onClick={go}
      disabled={pending || subscribed}
      className="w-full rounded-lg bg-brand px-4 py-2 text-xs font-bold text-white shadow-xs transition hover:bg-brand-dark disabled:opacity-50"
    >
      {subscribed ? "✓ Gói hiện tại" : pending ? "Đang đăng ký..." : "Đăng ký"}
    </button>
  );
}
