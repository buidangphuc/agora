"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Tag } from "@/components/ui/Tag";
import { DiscountType } from "@/generated/platform/promotion/v1/promotion_pb.js";
import type { MyVoucherRow } from "@/lib/gateway/promotion";
import { deactivateShopVoucherAction } from "../actions";
import { usePending } from "../usePending";
import { ConfirmDialog } from "./ConfirmDialog";
import { ShopVoucherForm } from "./ShopVoucherForm";

const day = (iso: string) => new Date(iso).toLocaleDateString("vi-VN");

function discountText(r: MyVoucherRow): string {
  return r.discountType === DiscountType.PERCENT
    ? `${r.discountValue}%`
    : `${r.discountValue.toLocaleString("vi-VN")}₫`;
}

export function VoucherTab({ rows }: { rows: MyVoucherRow[] }) {
  const router = useRouter();
  const [target, setTarget] = useState<string | null>(null);
  const [error, setError] = useState("");
  const { pending, run } = usePending();

  async function confirm() {
    if (!target) return;
    const res = await run(() => deactivateShopVoucherAction(target));
    setTarget(null);
    if (res.ok) {
      setError("");
      router.refresh();
    } else {
      setError(res.error);
    }
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Tạo voucher</CardTitle>
        </CardHeader>
        <CardContent>
          <ShopVoucherForm />
        </CardContent>
      </Card>
      {error && <Alert type="error" description={error} />}
      {rows.length === 0 ? (
        <Empty description="Shop chưa có voucher nào." />
      ) : (
        <ul data-testid="marketing-list" className="space-y-3">
          {rows.map((r) => (
            <li
              key={r.id}
              data-testid="voucher-row"
              className="flex flex-col gap-2 rounded-lg border border-border-subtle p-3 sm:flex-row sm:items-center sm:justify-between"
            >
              <div className="space-y-1">
                <p className="font-medium">{r.code}</p>
                <p className="text-sm text-text-secondary">
                  {discountText(r)} · {day(r.startsAt)} – {day(r.endsAt)}
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-3">
                <Tag>{r.stateText}</Tag>
                <span>{`${r.used}/${r.quota}`}</span>
                {r.active && r.state !== "ended" && (
                  <Button variant="outline" onClick={() => setTarget(r.id)}>
                    Tắt voucher
                  </Button>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
      <ConfirmDialog
        open={target !== null}
        title="Tắt voucher này?"
        description="Voucher sẽ không còn dùng được."
        pending={pending}
        onConfirm={confirm}
        onCancel={() => setTarget(null)}
      />
    </div>
  );
}
