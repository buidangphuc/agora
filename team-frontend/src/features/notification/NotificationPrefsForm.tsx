"use client";

import { type FormEvent, useState } from "react";

import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader } from "@/components/ui/Card";
import { Checkbox } from "@/components/ui/Checkbox";
import { FormItem } from "@/components/ui/FormItem";
import { Select } from "@/components/ui/Select";
import { useToast } from "@/components/ui/ToastProvider";
import { usePendingAction } from "@/features/account/usePendingAction";
import { DigestFrequency } from "@/generated/platform/notification/v1/notification_pb.js";
import type { ViewNotificationPrefs } from "@/lib/gateway/notification";
import { updateNotificationPrefsAction } from "./actions";

// Notification categories the user can toggle. Keyed by a stable string the
// backend stores in NotificationPrefs.typeEnabled.
const TYPE_ROWS: { key: string; label: string }[] = [
  { key: "ORDER", label: "Cập nhật đơn hàng" },
  { key: "PROMOTION", label: "Khuyến mãi & ưu đãi" },
  { key: "CHAT", label: "Tin nhắn từ shop" },
  { key: "PRICE_DROP", label: "Báo giảm giá" },
  { key: "BACK_IN_STOCK", label: "Có hàng trở lại" },
];

const DIGEST_OPTIONS = [
  { value: String(DigestFrequency.OFF), label: "Tắt (thông báo tức thời)" },
  { value: String(DigestFrequency.DAILY), label: "Tổng hợp hằng ngày" },
  { value: String(DigestFrequency.WEEKLY), label: "Tổng hợp hằng tuần" },
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
  const { pending, run } = usePendingAction();
  const toast = useToast();

  // On failure the toggles keep the unsaved values; nothing is reset here.
  function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void run(async () => {
      const res = await updateNotificationPrefsAction(enabled, digest);
      if (res.ok) toast.success("Đã lưu tùy chọn thông báo.");
      else toast.error(res.error);
    });
  }

  return (
    <Card>
      <CardHeader className="flex-col items-start justify-start gap-0">
        <h2 className="text-base font-semibold text-text-primary">
          Tùy chọn thông báo
        </h2>
        <p className="mt-0.5 text-xs text-text-secondary">
          Chọn loại thông báo bạn muốn nhận và tần suất tổng hợp.
        </p>
      </CardHeader>
      <CardContent>
        <form onSubmit={save} className="space-y-4">
          <fieldset className="space-y-3">
            <legend className="sr-only">Loại thông báo</legend>
            {TYPE_ROWS.map((row) => (
              <Checkbox
                key={row.key}
                name={row.key}
                label={row.label}
                checked={enabled[row.key]}
                onChange={(e) =>
                  setEnabled((prev) => ({
                    ...prev,
                    [row.key]: e.target.checked,
                  }))
                }
              />
            ))}
          </fieldset>

          <FormItem label="Tần suất tổng hợp">
            <Select
              name="digest"
              value={String(digest)}
              onChange={(e) =>
                setDigest(Number(e.target.value) as DigestFrequency)
              }
              options={DIGEST_OPTIONS}
            />
          </FormItem>

          <Button
            type="submit"
            isLoading={pending}
            className="min-h-10 w-full sm:w-auto"
          >
            Lưu tùy chọn
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
