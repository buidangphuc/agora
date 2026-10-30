import { Badge, type BadgeVariant } from "@/components/ui/Badge";
import { Descriptions } from "@/components/ui/Descriptions";
import type { VoucherView } from "@/lib/gateway/vouchers";

import {
  SCOPE_LABEL,
  STATE_LABEL,
  type VoucherState,
  describeDiscount,
  describeMinSpend,
  describeUsage,
  formatVnDateTime,
  voucherState,
} from "./voucherFormat";

const STATE_VARIANT: Record<VoucherState, BadgeVariant> = {
  upcoming: "neutral",
  active: "success",
  ended: "neutral",
  exhausted: "warning",
};

export function VoucherStateBadge({
  voucher,
  now,
}: {
  voucher: VoucherView;
  now: Date;
}) {
  const state = voucherState(voucher, now);
  return (
    <Badge variant={STATE_VARIANT[state]} data-testid="voucher-state">
      {STATE_LABEL[state]}
    </Badge>
  );
}

export function VoucherDetails({
  voucher,
  now,
  title,
}: {
  voucher: VoucherView;
  now: Date;
  title?: string;
}) {
  return (
    <div data-testid="voucher-details">
      <Descriptions
        title={title}
        bordered
        column={2}
        items={[
          { key: "code", label: "Mã voucher", children: voucher.code },
          {
            key: "state",
            label: "Trạng thái",
            children: <VoucherStateBadge voucher={voucher} now={now} />,
          },
          {
            key: "scope",
            label: "Phạm vi",
            children: SCOPE_LABEL[voucher.scope],
          },
          {
            key: "discount",
            label: "Mức giảm",
            children: describeDiscount(voucher),
          },
          {
            key: "minSpend",
            label: "Đơn tối thiểu",
            children: describeMinSpend(voucher.minSpend),
          },
          {
            key: "usage",
            label: "Đã dùng / lượt tối đa",
            children: describeUsage(voucher),
          },
          {
            key: "startsAt",
            label: "Bắt đầu (giờ Việt Nam)",
            children: formatVnDateTime(voucher.startsAt),
          },
          {
            key: "endsAt",
            label: "Kết thúc (giờ Việt Nam)",
            children: formatVnDateTime(voucher.endsAt),
          },
          { key: "id", label: "ID", children: voucher.id },
        ]}
      />
    </div>
  );
}
