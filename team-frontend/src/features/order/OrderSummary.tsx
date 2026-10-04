import type { ReactNode } from "react";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Descriptions } from "@/components/ui/Descriptions";
import { PriceTag } from "@/components/ui/PriceTag";
import { Tag } from "@/components/ui/Tag";
import { formatPrice } from "@/components/ui/format";
import { MobileSummaryBar } from "./MobileSummaryBar";
import { orderTotal } from "./orderTotal";
import type { ShippingFee } from "./shipping";

export interface OrderSummaryProps {
  itemCount: number;
  subtotal: number;
  /** Server-previewed voucher discount (0 when none). */
  discount: number;
  voucherCode?: string;
  /** null while no address is chosen yet (cart): the fee is shown as pending. */
  shipping: ShippingFee | null;
  /** Primary action of the card (desktop). */
  action: ReactNode;
  /** Compact action for the mobile bottom bar (defaults to `action`). */
  mobileAction?: ReactNode;
  /** Optional notice above the action (kill-switch, saga failure slot). */
  notice?: ReactNode;
}

function SummaryRows({
  itemCount,
  subtotal,
  discount,
  voucherCode,
  shipping,
}: Omit<OrderSummaryProps, "action" | "mobileAction" | "notice">) {
  const total = orderTotal(subtotal, discount, shipping?.fee ?? 0);
  return (
    <Descriptions
      bordered={false}
      column={1}
      items={[
        {
          key: "subtotal",
          label: `Tạm tính (${itemCount} sản phẩm)`,
          children: formatPrice(subtotal),
        },
        {
          key: "discount",
          label: voucherCode ? (
            <span className="inline-flex items-center gap-1.5">
              Giảm giá <Tag color="success">{voucherCode}</Tag>
            </span>
          ) : (
            "Giảm giá"
          ),
          children: (
            <span data-testid="voucher-discount">
              {discount > 0 ? `-${formatPrice(discount)}` : "-"}
            </span>
          ),
        },
        {
          key: "shipping",
          label: "Phí vận chuyển",
          children:
            shipping === null ? (
              <span className="font-normal text-text-secondary">
                Tính ở bước thanh toán
              </span>
            ) : shipping.isFree ? (
              <Tag color="success">Freeship</Tag>
            ) : (
              formatPrice(shipping.fee)
            ),
        },
        {
          key: "total",
          label: "Tổng cộng",
          children: (
            <span data-testid="order-total">
              <PriceTag price={total} size="lg" />
            </span>
          ),
        },
      ]}
    />
  );
}

/**
 * Same rows in the same positions in every step (discount always renders), so
 * a changed selection only changes text. Desktop: sticky card. Mobile: bottom
 * bar + Drawer.
 */
export function OrderSummary({
  action,
  mobileAction,
  notice,
  ...rows
}: OrderSummaryProps) {
  const total = orderTotal(
    rows.subtotal,
    rows.discount,
    rows.shipping?.fee ?? 0,
  );
  return (
    <>
      <Card
        className="hidden lg:sticky lg:top-4 lg:block"
        data-testid="order-summary"
      >
        <CardHeader>
          <CardTitle>Tóm tắt đơn hàng</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <SummaryRows {...rows} />
          {notice}
          {action}
        </CardContent>
      </Card>
      <MobileSummaryBar total={total} action={mobileAction ?? action}>
        <SummaryRows {...rows} />
      </MobileSummaryBar>
    </>
  );
}
