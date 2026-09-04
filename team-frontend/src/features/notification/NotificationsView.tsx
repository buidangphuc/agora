"use client";

import Link from "next/link";
import { useState } from "react";

import { NotificationType } from "@/generated/platform/notification/v1/notification_pb.js";
import type {
  ViewNotification,
  ViewNotificationPrefs,
} from "@/lib/gateway/notification";
import {
  type AlertSubscriptionRow,
  AlertSubscriptions,
} from "./AlertSubscriptions";
import { NotificationPrefsForm } from "./NotificationPrefsForm";

function typeMeta(type: NotificationType): {
  icon: string;
  group: "order" | "chat" | "alert" | "system";
} {
  switch (type) {
    case NotificationType.ORDER:
      return { icon: "🏷️", group: "order" };
    case NotificationType.CHAT:
      return { icon: "💬", group: "chat" };
    case NotificationType.PROMOTION:
      return { icon: "🎉", group: "alert" };
    case NotificationType.PRICE_DROP:
      return { icon: "📉", group: "alert" };
    case NotificationType.BACK_IN_STOCK:
      return { icon: "📦", group: "alert" };
    default:
      return { icon: "📜", group: "system" };
  }
}

const TABS: { key: string; label: string }[] = [
  { key: "all", label: "Tất cả" },
  { key: "order", label: "🏷️ Đơn hàng" },
  { key: "chat", label: "💬 Tin nhắn" },
  { key: "alert", label: "🔔 Giá & Kho hàng" },
  { key: "system", label: "📜 Hệ thống" },
];

export function NotificationsView({
  notifications,
  subscriptions,
  prefs,
}: {
  notifications: ViewNotification[];
  subscriptions: AlertSubscriptionRow[];
  prefs: ViewNotificationPrefs;
}) {
  const [tab, setTab] = useState<string>("all");
  const [items, setItems] = useState<ViewNotification[]>(notifications);

  function handleMarkAllRead() {
    setItems((prev) => prev.map((n) => ({ ...n, isRead: true })));
  }

  const filtered =
    tab === "all" ? items : items.filter((n) => typeMeta(n.type).group === tab);

  return (
    <div className="space-y-6">
      {/* ── Header ── */}
      <div className="flex items-center justify-between border-b bg-white p-5 rounded-2xl shadow-2xs">
        <div>
          <h1 className="text-lg font-bold text-gray-900 flex items-center gap-2">
            <span>🔔</span>
            <span>Trung tâm thông báo</span>
          </h1>
          <p className="text-xs text-gray-500 mt-0.5">
            Cập nhật đơn hàng, khuyến mãi, biến động giá và tình trạng kho hàng
          </p>
        </div>

        <button
          type="button"
          onClick={handleMarkAllRead}
          className="text-xs font-semibold text-red-600 hover:underline"
        >
          ✓ Đánh dấu tất cả đã đọc
        </button>
      </div>

      {/* ── Notification preferences (per-type toggles + digest) ── */}
      <NotificationPrefsForm initial={prefs} />

      {/* ── Alert subscriptions management ── */}
      <AlertSubscriptions initial={subscriptions} />

      {/* ── Tabs ── */}
      <div className="flex flex-wrap border-b border-gray-200 bg-white p-2 rounded-xl shadow-2xs text-xs font-semibold text-gray-600 gap-2">
        {TABS.map((t) => (
          <button
            key={t.key}
            type="button"
            onClick={() => setTab(t.key)}
            className={`py-2 px-4 rounded-lg transition ${
              tab === t.key
                ? "bg-red-600 text-white font-bold shadow-2xs"
                : "hover:bg-gray-100"
            }`}
          >
            {t.key === "all" ? `Tất cả (${items.length})` : t.label}
          </button>
        ))}
      </div>

      {/* ── Notification List ── */}
      {filtered.length === 0 ? (
        <div className="rounded-2xl border border-gray-100 bg-white p-10 text-center text-xs text-gray-400 shadow-2xs">
          Chưa có thông báo nào trong mục này.
        </div>
      ) : (
        <div className="divide-y rounded-2xl border border-gray-100 bg-white shadow-2xs overflow-hidden">
          {filtered.map((n) => {
            const meta = typeMeta(n.type);
            return (
              <Link
                key={n.id}
                href={n.linkUrl || "#"}
                data-testid="notification-item"
                data-type={
                  NotificationType[n.type]?.toLowerCase?.() ?? "system"
                }
                className={`flex items-start gap-4 p-4 transition hover:bg-gray-50/80 ${
                  !n.isRead ? "bg-red-50/30" : ""
                }`}
              >
                <div className="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-red-100 text-lg text-red-600">
                  {meta.icon}
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center justify-between gap-2">
                    <h3 className="text-xs font-bold text-gray-900">
                      {n.title}
                    </h3>
                    <span className="text-[10px] text-gray-400 shrink-0">
                      {n.createdAt}
                    </span>
                  </div>
                  <p className="mt-1 text-xs text-gray-600 leading-relaxed">
                    {n.body}
                  </p>
                </div>
                {!n.isRead && (
                  <span className="h-2.5 w-2.5 shrink-0 rounded-full bg-red-600 self-center" />
                )}
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}
