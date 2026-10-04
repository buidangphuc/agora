import Link from "next/link";

import { Card } from "@/components/ui/Card";
import { Progress } from "@/components/ui/Progress";
import { Tag } from "@/components/ui/Tag";
import { formatPrice } from "@/components/ui/format";
import { linkButtonClass } from "@/features/home/linkButton";
import { DiscountType, type ViewVoucher } from "@/lib/gateway/promotion";

function title(v: ViewVoucher): string {
  if (v.discountType === DiscountType.PERCENT) {
    const cap =
      v.maxDiscount > 0 ? ` tối đa ${formatPrice(v.maxDiscount)}` : "";
    return `Giảm ${v.discountValue}%${cap}`;
  }
  return `Giảm ${formatPrice(v.discountValue)}`;
}

/**
 * One voucher from team-promotion. Only real figures are shown: the discount,
 * minimum spend, expiry and a usage bar from the real used / quota (no bar for
 * an unlimited quota). There is no "save" control: no backend can claim a code.
 */
export function VoucherCard({ voucher: v }: { voucher: ViewVoucher }) {
  const limited = v.quota > 0;
  return (
    <Card
      data-testid="voucher-card"
      data-code={v.code}
      hoverable
      className="flex flex-col gap-3 p-4"
    >
      <div className="flex flex-wrap items-center gap-2">
        <Tag color="primary">{v.code}</Tag>
        <Tag color="neutral">{v.discountTypeText}</Tag>
        <Tag color="neutral">{v.scopeText}</Tag>
      </div>

      <div className="min-w-0">
        <h3 className="text-base font-semibold text-text-primary">
          {title(v)}
        </h3>
        <p className="mt-0.5 text-sm text-text-secondary">
          {v.minSpend > 0
            ? `Cho đơn từ ${formatPrice(v.minSpend)}`
            : "Không yêu cầu đơn tối thiểu"}
        </p>
        {v.endsAt && (
          <p className="mt-0.5 text-xs text-text-secondary">HSD: {v.endsAt}</p>
        )}
      </div>

      <div className="mt-auto flex items-center justify-between gap-4 border-t border-border-subtle pt-3">
        <div className="min-w-0 flex-1">
          {limited && (
            <>
              <p className="mb-1 text-xs text-text-secondary">
                Đã dùng {v.used}/{v.quota}
              </p>
              <Progress
                size="sm"
                showInfo={false}
                label={`Đã dùng ${v.code}`}
                percent={(v.used / v.quota) * 100}
              />
            </>
          )}
        </div>
        <Link
          href="/search"
          className={`shrink-0 ${linkButtonClass("primary")}`}
        >
          Dùng ngay
        </Link>
      </div>
    </Card>
  );
}
