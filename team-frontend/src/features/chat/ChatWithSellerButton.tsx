"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { getOrCreateThreadAction } from "./actions";

export function ChatWithSellerButton({
  sellerId,
  listingId,
  loggedIn,
}: {
  sellerId: string;
  listingId: string;
  loggedIn: boolean;
}) {
  const router = useRouter();
  const [loading, setLoading] = useState(false);

  async function handleChat() {
    if (!loggedIn) {
      router.push(`/login?returnUrl=/listing/${listingId}`);
      return;
    }
    setLoading(true);
    try {
      const res = await getOrCreateThreadAction(sellerId, listingId);
      if (res.ok && res.threadId) {
        router.push(`/chat?thread=${res.threadId}`);
      } else {
        alert(res.message || "Không thể kết nối với người bán.");
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <button
      type="button"
      disabled={loading}
      onClick={handleChat}
      className="inline-flex items-center gap-1.5 rounded-lg border border-brand bg-orange-50 px-4 py-2 text-sm font-semibold text-brand transition hover:bg-orange-100 disabled:opacity-60"
    >
      💬 {loading ? "Đang kết nối..." : "Chat với người bán"}
    </button>
  );
}
