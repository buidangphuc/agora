"use client";

import Link from "next/link";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Tag } from "@/components/ui/Tag";
import { useToast } from "@/components/ui/ToastProvider";
import { usePendingAction } from "@/features/account/usePendingAction";
import { AlertType } from "@/generated/platform/notification/v1/notification_pb.js";
import { removeAlertSubscriptionAction } from "./actions";

export interface AlertSubscriptionRow {
  id: string;
  listingId: string;
  type: AlertType;
  title: string;
}

function typeLabel(type: AlertType): string {
  switch (type) {
    case AlertType.PRICE_DROP:
      return "Giảm giá";
    case AlertType.BACK_IN_STOCK:
      return "Có hàng lại";
    default:
      return "Thông báo";
  }
}

function dataType(type: AlertType): string {
  switch (type) {
    case AlertType.PRICE_DROP:
      return "price_drop";
    case AlertType.BACK_IN_STOCK:
      return "back_in_stock";
    default:
      return "unknown";
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
  const [removing, setRemoving] = useState<string | null>(null);
  const { pending, run } = usePendingAction();
  const toast = useToast();

  function remove(id: string, listingId: string) {
    setRemoving(id);
    void run(async () => {
      const res = await removeAlertSubscriptionAction(id, listingId);
      if (res.ok) {
        setRows((prev) => prev.filter((r) => r.id !== id));
        toast.info("Đã hủy theo dõi thông báo.");
      } else {
        toast.error(res.error);
      }
    }).finally(() => setRemoving(null));
  }

  return (
    <Card>
      <CardHeader className="flex-col items-start justify-start gap-0">
        <h2 className="text-base font-semibold text-text-primary">
          Thông báo đang theo dõi
        </h2>
        <p className="mt-0.5 text-xs text-text-secondary">
          Sản phẩm bạn đã bật báo giảm giá hoặc có hàng lại.
        </p>
      </CardHeader>
      {rows.length === 0 ? (
        <Empty description='Bạn chưa theo dõi thông báo cho sản phẩm nào. Mở một sản phẩm và bật "Báo tôi khi giảm giá / có hàng lại".' />
      ) : (
        <CardContent className="py-2">
          <ul className="divide-y divide-border-subtle">
            {rows.map((r) => (
              <li
                key={r.id}
                data-testid="alert-subscription"
                data-type={dataType(r.type)}
                className="flex items-center justify-between gap-3 py-3"
              >
                <div className="min-w-0 space-y-1">
                  <Link
                    href={`/listing/${r.listingId}`}
                    className="block truncate text-sm font-medium text-text-primary hover:text-action-primary"
                  >
                    {r.title}
                  </Link>
                  <Tag>{typeLabel(r.type)}</Tag>
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  className="min-h-10 shrink-0"
                  isLoading={pending && removing === r.id}
                  disabled={pending}
                  onClick={() => remove(r.id, r.listingId)}
                >
                  Hủy
                </Button>
              </li>
            ))}
          </ul>
        </CardContent>
      )}
    </Card>
  );
}
