"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import {
  subscribeAlertAction,
  unsubscribeAlertAction,
} from "@/features/notification/actions";
import { AlertType } from "@/generated/platform/notification/v1/notification_pb.js";
import { trackEcommerce } from "@/lib/analytics";

/**
 * "Notify me" toggles on the listing page: price-drop and back-in-stock.
 * Each is an independent subscribe/unsubscribe against team-notification
 * (gateway-only, via server actions). Rendered by the listing slot.
 */
export interface AlertState {
  priceDropSubId?: string;
  backInStockSubId?: string;
}

const TYPE_META: Record<
  "price_drop" | "back_in_stock",
  { type: AlertType; label: string; icon: string }
> = {
  price_drop: { type: AlertType.PRICE_DROP, label: "Giảm giá", icon: "📉" },
  back_in_stock: {
    type: AlertType.BACK_IN_STOCK,
    label: "Có hàng lại",
    icon: "📦",
  },
};

function ToggleButton({
  listingId,
  kind,
  initialSubId,
  loggedIn,
}: {
  listingId: string;
  kind: "price_drop" | "back_in_stock";
  initialSubId?: string;
  loggedIn: boolean;
}) {
  const meta = TYPE_META[kind];
  const [subId, setSubId] = useState<string | undefined>(initialSubId);
  const [pending, start] = useTransition();
  const toast = useToast();
  const active = Boolean(subId);

  function toggle() {
    if (!loggedIn) {
      toast.info("Vui lòng đăng nhập để nhận thông báo.");
      return;
    }
    start(async () => {
      if (active && subId) {
        const res = await unsubscribeAlertAction(subId, listingId);
        if (res.ok) {
          setSubId(undefined);
          toast.info(`Đã tắt thông báo ${meta.label.toLowerCase()}.`);
        } else {
          toast.error(res.message || "Hủy thông báo thất bại.");
        }
      } else {
        const res = await subscribeAlertAction(listingId, meta.type);
        if (res.ok) {
          setSubId(res.subscriptionId || "pending");
          trackEcommerce("alert_subscribe", {
            items: [{ itemId: listingId }],
            properties: { alert_type: kind },
          });
          toast.success(`✓ Sẽ báo khi ${meta.label.toLowerCase()}!`);
        } else {
          toast.error(res.message || "Đăng ký thông báo thất bại.");
        }
      }
    });
  }

  return (
    <button
      type="button"
      data-testid="alert-toggle"
      data-type={kind}
      data-active={active ? "true" : "false"}
      aria-pressed={active}
      onClick={toggle}
      disabled={pending}
      className={`inline-flex min-h-touch items-center gap-2 rounded-xl border px-4 text-sm font-medium transition disabled:opacity-60 ${
        active
          ? "border-action-primary bg-primary-50 text-action-primary"
          : "border-border-strong text-text-secondary hover:border-action-primary hover:text-action-primary"
      }`}
    >
      <span>{meta.icon}</span>
      <span>
        {active ? "Đang theo dõi" : "Báo tôi khi"} {meta.label.toLowerCase()}
      </span>
    </button>
  );
}

export function AlertToggle({
  listingId,
  initial,
  loggedIn,
}: {
  listingId: string;
  initial: AlertState;
  loggedIn: boolean;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="w-full text-sm text-text-secondary sm:w-auto">
        🔔 Nhận thông báo:
      </span>
      <ToggleButton
        listingId={listingId}
        kind="price_drop"
        initialSubId={initial.priceDropSubId}
        loggedIn={loggedIn}
      />
      <ToggleButton
        listingId={listingId}
        kind="back_in_stock"
        initialSubId={initial.backInStockSubId}
        loggedIn={loggedIn}
      />
    </div>
  );
}
