"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { DigestFrequency } from "@/generated/platform/notification/v1/notification_pb.js";
import type { ViewNotificationPrefs } from "@/lib/gateway/notification";
import { updateNotificationPrefsAction } from "./actions";

// Notification categories the user can toggle. Keyed by a stable string the
// backend stores in NotificationPrefs.typeEnabled.
const TYPE_ROWS: { key: string; label: string }[] = [
  { key: "ORDER", label: "🏷️ Cập nhật đơn hàng" },
  { key: "PROMOTION", label: "🎉 Khuyến mãi & ưu đãi" },
  { key: "CHAT", label: "💬 Tin nhắn từ shop" },
  { key: "PRICE_DROP", label: "📉 Báo giảm giá" },
  { key: "BACK_IN_STOCK", label: "📦 Có hàng trở lại" },
];

const DIGEST_OPTIONS: { value: DigestFrequency; label: string }[] = [
  { value: DigestFrequency.OFF, label: "Tắt (thông báo tức thời)" },
  { value: DigestFrequency.DAILY, label: "Tổng hợp hằng ngày" },
  { value: DigestFrequency.WEEKLY, label: "Tổng hợp hằng tuần" },
];

export function NotificationPrefsForm({
  initial,
}: {
  initial: ViewNotificationPrefs;
}) {
  const [enabled, setEnabled] = useState<Record<string, boolean>>(() => {
    const seed: Record<string, boolean> = {};
    for (const row of TYPE_ROWS) {
      seed[row.key] = initial.typeEnabled[row.key] ?? true;
    }
    return seed;
  });
  const [digest, setDigest] = useState<DigestFrequency>(initial.digestFreq);
  const [pending, start] = useTransition();
  const toast = useToast();

  function save() {
    start(async () => {
      const res = await updateNotificationPrefsAction(enabled, digest);
      if (res.ok) toast.success("✓ Đã lưu tùy chọn thông báo.");
      else toast.error(res.message || "Lưu thất bại.");
    });
  }

  return (
    <div className="space-y-3 rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs">
      <div>
        <h2 className="flex items-center gap-2 text-base font-bold text-gray-900">
          <span>⚙️</span>
          <span>Tùy chọn thông báo</span>
        </h2>
        <p className="mt-0.5 text-xs text-gray-500">
          Chọn loại thông báo bạn muốn nhận và tần suất tổng hợp.
        </p>
      </div>

      <ul className="divide-y divide-gray-100">
        {TYPE_ROWS.map((row) => (
          <li
            key={row.key}
            className="flex items-center justify-between py-2.5 text-sm text-gray-700"
          >
            <span>{row.label}</span>
            <input
              type="checkbox"
              checked={enabled[row.key]}
              onChange={(e) =>
                setEnabled((prev) => ({ ...prev, [row.key]: e.target.checked }))
              }
              className="h-4 w-4 accent-brand"
            />
          </li>
        ))}
      </ul>

      <div className="flex items-center justify-between gap-3 pt-1">
        <label htmlFor="digest-freq" className="text-sm text-gray-700">
          Tần suất tổng hợp
        </label>
        <select
          id="digest-freq"
          value={digest}
          onChange={(e) => setDigest(Number(e.target.value) as DigestFrequency)}
          className="rounded-lg border border-gray-200 px-3 py-1.5 text-xs"
        >
          {DIGEST_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </div>

      <button
        type="button"
        onClick={save}
        disabled={pending}
        className="rounded-lg bg-brand px-4 py-2 text-xs font-bold text-white shadow-xs transition hover:bg-brand-dark disabled:opacity-60"
      >
        {pending ? "Đang lưu..." : "Lưu tùy chọn"}
      </button>
    </div>
  );
}
