"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { revokeSessionAction } from "./actions";

/**
 * Revoke one active session. Wired to team-identity SessionService through the
 * gateway via a server action. Thin — styling to be revamped later.
 */
export function RevokeSessionButton({ sessionId }: { sessionId: string }) {
  const [revoked, setRevoked] = useState(false);
  const [pending, start] = useTransition();
  const toast = useToast();

  function revoke() {
    start(async () => {
      const res = await revokeSessionAction(sessionId);
      if (res.ok) {
        setRevoked(true);
        toast.success("✓ Đã thu hồi phiên đăng nhập.");
      } else {
        toast.error(res.message || "Có lỗi xảy ra.");
      }
    });
  }

  if (revoked) {
    return <span className="text-xs text-gray-400">Đã thu hồi</span>;
  }

  return (
    <button
      type="button"
      onClick={revoke}
      disabled={pending}
      className="rounded-md border border-red-200 bg-white px-3 py-1 text-xs font-medium text-red-600 hover:bg-red-50 disabled:opacity-50"
    >
      {pending ? "Đang thu hồi..." : "Thu hồi"}
    </button>
  );
}
