"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { followSellerAction, unfollowSellerAction } from "./actions";

/**
 * Follow / Unfollow a shop. Optimistic toggle wired to team-engagement through
 * the gateway via server actions. Thin — styling to be revamped later.
 */
export function FollowSellerButton({
  sellerId,
  initialFollowing,
}: {
  sellerId: string;
  initialFollowing: boolean;
}) {
  const [following, setFollowing] = useState(initialFollowing);
  const [pending, start] = useTransition();
  const toast = useToast();

  function toggle() {
    const next = !following;
    setFollowing(next);
    start(async () => {
      const res = next
        ? await followSellerAction(sellerId)
        : await unfollowSellerAction(sellerId);
      if (res.ok) {
        toast.success(next ? "✓ Đã theo dõi shop." : "Đã bỏ theo dõi shop.");
      } else {
        setFollowing(!next);
        toast.error(res.message || "Có lỗi xảy ra.");
      }
    });
  }

  return (
    <button
      type="button"
      onClick={toggle}
      disabled={pending}
      className={`rounded-lg px-3 py-1.5 text-xs font-semibold transition disabled:opacity-60 ${
        following
          ? "bg-white/20 text-white"
          : "bg-white text-brand hover:bg-yellow-100"
      }`}
    >
      {following ? "✓ Đang theo dõi" : "+ Theo dõi"}
    </button>
  );
}
