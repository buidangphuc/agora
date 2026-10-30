"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Tag } from "@/components/ui/Tag";
import { formatPrice } from "@/components/ui/format";
import { CheckoutVoucherScope } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewCheckoutGroup, ViewVoucherQuote } from "@/lib/gateway/orders";
import type { ViewVoucher } from "@/lib/gateway/promotion";
import { type CheckoutState, checkoutHref } from "./checkoutParams";

function amount(n: number): string {
  return n > 0 ? formatPrice(n) : "-";
}

function Slot({
  testId,
  label,
  appliedCode,
  quote,
  available,
  onApply,
  onRemove,
}: {
  testId: string;
  label: string;
  appliedCode?: string;
  quote?: ViewVoucherQuote;
  available: ViewVoucher[];
  onApply: (code: string) => void;
  onRemove: () => void;
}) {
  const [code, setCode] = useState("");
  const trimmed = code.trim();
  return (
    <div data-testid={testId} className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-semibold text-text-primary">{label}</span>
        {appliedCode && (
          <Tag
            color={quote && !quote.valid ? "danger" : "success"}
            closable
            closeLabel={`Bỏ mã ${appliedCode}`}
            onClose={onRemove}
          >
            {appliedCode}
          </Tag>
        )}
      </div>
      <form
        className="flex items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (!trimmed) return;
          onApply(trimmed);
          setCode("");
        }}
      >
        <Input
          name="voucher_code"
          label={label}
          value={code}
          placeholder="Nhập mã giảm giá"
          autoComplete="off"
          onChange={(e) => setCode(e.target.value)}
        />
        <Button type="submit" size="sm" disabled={!trimmed}>
          Áp dụng
        </Button>
      </form>
      {available.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {available.map((v) => (
            <Button
              key={v.code}
              type="button"
              variant="outline"
              size="sm"
              onClick={() => onApply(v.code)}
            >
              {v.code}
            </Button>
          ))}
        </div>
      )}
      {quote && !quote.valid && quote.reason && (
        <p role="alert" className="text-xs text-danger">
          {quote.reason}
        </p>
      )}
    </div>
  );
}

export function VoucherModal({
  state,
  groups,
  quotes,
  available = {},
  platformAvailable = [],
}: {
  state: CheckoutState;
  groups: ViewCheckoutGroup[];
  quotes: ViewVoucherQuote[];
  available?: Record<string, ViewVoucher[]>;
  platformAvailable?: ViewVoucher[];
}) {
  const router = useRouter();

  function go(next: Partial<CheckoutState>) {
    router.replace(checkoutHref({ ...state, ...next }));
  }

  function shopQuote(sellerId: string, code?: string) {
    return quotes.find(
      (q) =>
        q.scope === CheckoutVoucherScope.SHOP &&
        q.sellerId === sellerId &&
        q.code === code,
    );
  }

  const platformQuote = quotes.find(
    (q) => q.scope === CheckoutVoucherScope.PLATFORM && q.code === state.pv,
  );
  const platformInvalid = platformQuote ? !platformQuote.valid : false;

  return (
    <Card>
      <div className="space-y-4 p-4">
        {groups.map((g) => {
          const code = state.sv[g.sellerId];
          const invalid = code ? !shopQuote(g.sellerId, code)?.valid : false;
          return (
            <div key={g.sellerId} className="space-y-1">
              <Slot
                testId={`shop-voucher-${g.sellerId}`}
                label={`Mã giảm giá của shop ${g.sellerId}`}
                appliedCode={code}
                quote={shopQuote(g.sellerId, code)}
                available={available[g.sellerId] ?? []}
                onApply={(c) => go({ sv: { ...state.sv, [g.sellerId]: c } })}
                onRemove={() => {
                  const { [g.sellerId]: _drop, ...rest } = state.sv;
                  go({ sv: rest });
                }}
              />
              <p className="flex justify-between text-xs text-text-secondary">
                <span>Giảm giá shop</span>
                <span data-testid={`shop-discount-${g.sellerId}`}>
                  {invalid ? "-" : amount(g.shopDiscountAmount)}
                </span>
              </p>
              <p className="flex justify-between text-xs text-text-secondary">
                <span>Phần giảm giá sàn</span>
                <span data-testid={`platform-share-${g.sellerId}`}>
                  {platformInvalid ? "-" : amount(g.platformDiscountAmount)}
                </span>
              </p>
            </div>
          );
        })}
        <Slot
          testId="platform-voucher"
          label="Mã giảm giá của sàn"
          appliedCode={state.pv}
          quote={platformQuote}
          available={platformAvailable}
          onApply={(c) => go({ pv: c })}
          onRemove={() => go({ pv: undefined })}
        />
      </div>
    </Card>
  );
}
