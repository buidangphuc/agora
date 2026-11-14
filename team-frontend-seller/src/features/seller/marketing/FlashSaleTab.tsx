"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Tag } from "@/components/ui/Tag";
import { formatPrice } from "@/components/ui/format";
import { CampaignStatus } from "@/generated/platform/promotion/v1/promotion_pb.js";
import type { MyCampaignRow } from "@/lib/gateway/promotion";
import { cancelCampaignAction } from "../actions";
import { usePending } from "../usePending";
import { ConfirmDialog } from "./ConfirmDialog";
import { FlashSaleForm } from "./FlashSaleForm";

const day = (iso: string) => new Date(iso).toLocaleDateString("vi-VN");

export function FlashSaleTab({
  rows,
  titles,
}: {
  rows: MyCampaignRow[];
  titles: Record<string, string>;
}) {
  const router = useRouter();
  const [target, setTarget] = useState<string | null>(null);
  const [error, setError] = useState("");
  const { pending, run } = usePending();

  async function confirm() {
    if (!target) return;
    const res = await run(() => cancelCampaignAction(target));
    setTarget(null);
    if (res.ok) {
      setError("");
    } else {
      setError(res.error);
    }
    router.refresh();
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Tạo flash sale</CardTitle>
        </CardHeader>
        <CardContent>
          <FlashSaleForm />
        </CardContent>
      </Card>
      {error && <Alert type="error" description={error} />}
      {rows.length === 0 ? (
        <Empty description="Shop chưa có flash sale nào." />
      ) : (
        <ul data-testid="marketing-list" className="space-y-3">
          {rows.map((r) => (
            <li
              key={r.id}
              data-testid="campaign-row"
              className="flex flex-col gap-2 rounded-lg border border-border-subtle p-3 sm:flex-row sm:items-center sm:justify-between"
            >
              <div className="space-y-1">
                <p className="font-medium">
                  <span>{titles[r.listingId] ?? r.listingId}</span>
                </p>
                <p className="text-sm text-text-secondary">
                  {formatPrice(r.salePrice)} · {day(r.startsAt)} –{" "}
                  {day(r.endsAt)}
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-3">
                <Tag>{r.statusText}</Tag>
                <span>{r.soldText}</span>
                {r.status === CampaignStatus.SCHEDULED && (
                  <Button variant="outline" onClick={() => setTarget(r.id)}>
                    Huỷ
                  </Button>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
      <ConfirmDialog
        open={target !== null}
        title="Huỷ flash sale này?"
        description="Flash sale sắp diễn ra sẽ bị huỷ."
        pending={pending}
        onConfirm={confirm}
        onCancel={() => setTarget(null)}
      />
    </div>
  );
}
