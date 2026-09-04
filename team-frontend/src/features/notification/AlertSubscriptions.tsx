"use client";

import Link from "next/link";
import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { AlertType } from "@/generated/platform/notification/v1/notification_pb.js";
import { unsubscribeAlertAction } from "./actions";

export interface AlertSubscriptionRow {
  id: string;
  listingId: string;
  type: AlertType;
  title: string;
}

function typeLabel(type: AlertType): { icon: string; text: string } {
  switch (type) {
    case AlertType.PRICE_DROP:
      return { icon: "📉", text: "Giảm giá" };
    case AlertType.BACK_IN_STOCK:
      return { icon: "📦", text: "Có hàng lại" };
    default:
      return { icon: "🔔", text: "Thông báo" };
  }
}

/**
 * Manage the user's price-drop / back-in-stock alert subscriptions from the
 * notifications area. Gateway-only via server actions.
 */
export function AlertSubscriptions({
  initial,
}: {
  initial: AlertSubscriptionRow[];
}) {
  const [rows, setRows] = useState<AlertSubscriptionRow[]>(initial);
  const [pending, start] = useTransition();
  const toast = useToast();

  function remove(id: string, listingId: string) {
    start(async () => {
      const res = await unsubscribeAlertAction(id, listingId);
      if (res.ok) {
        setRows((prev) => prev.filter((r) => r.id !== id));
        toast.info("Đã hủy theo dõi thông báo.");
      } else {
        toast.error(res.message || "Hủy thông báo thất bại.");
      }
    });
  }

  return (
    <div className="rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs space-y-3">
      <div>
        <h2 className="text-base font-bold text-gray-900 flex items-center gap-2">
          <span>🔔</span>
          <span>Thông báo đang theo dõi</span>
        </h2>
        <p className="text-xs text-gray-500 mt-0.5">
          Sản phẩm bạn đã bật báo giảm giá hoặc có hàng lại
        </p>
      </div>

      {rows.length === 0 ? (
        <p className="text-xs text-gray-400 py-2">
          Bạn chưa theo dõi thông báo cho sản phẩm nào. Mở một sản phẩm và bật
          "Báo tôi khi giảm giá / có hàng lại".
        </p>
      ) : (
        <ul className="divide-y divide-gray-100">
          {rows.map((r) => {
            const label = typeLabel(r.type);
            return (
              <li
                key={r.id}
                data-testid="alert-subscription"
                data-type={
                  r.type === AlertType.PRICE_DROP
                    ? "price_drop"
                    : r.type === AlertType.BACK_IN_STOCK
                      ? "back_in_stock"
                      : "unknown"
                }
                className="flex items-center justify-between gap-3 py-2.5"
              >
                <div className="min-w-0">
                  <Link
                    href={`/listing/${r.listingId}`}
                    className="block truncate text-sm font-medium text-gray-800 hover:text-brand"
                  >
                    {r.title}
                  </Link>
                  <span className="inline-flex items-center gap-1 text-[11px] text-gray-500">
                    <span>{label.icon}</span>
                    <span>{label.text}</span>
                  </span>
                </div>
                <button
                  type="button"
                  disabled={pending}
                  onClick={() => remove(r.id, r.listingId)}
                  className="shrink-0 rounded-lg border border-gray-200 px-3 py-1.5 text-xs font-medium text-gray-600 transition hover:border-red-300 hover:text-red-600 disabled:opacity-60"
                >
                  Hủy
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
