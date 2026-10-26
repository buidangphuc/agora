import { PriceTag } from "@/components/ui/PriceTag";
import { Tag } from "@/components/ui/Tag";
import { formatPrice } from "@/components/ui/format";
import type { ViewCartItem } from "@/lib/gateway/cart";
import type { ViewCheckoutGroup } from "@/lib/gateway/orders";
import { shopLabel } from "@/lib/gateway/shops";
import { formatDeliveryDate } from "./deliveryDate";

function Row({
  label,
  children,
}: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3 text-sm">
      <span className="text-text-secondary">{label}</span>
      <span>{children}</span>
    </div>
  );
}

export function CheckoutGroups({
  groups,
  items,
  shippingQuoted,
}: {
  groups: ViewCheckoutGroup[];
  items: ViewCartItem[];
  shippingQuoted: boolean;
}) {
  return (
    <div className="space-y-4">
      {groups.map((g) => {
        const lines = items.filter((it) => g.itemIds.includes(it.id));
        const shopName = lines.find(
          (it) => it.sellerDisplayName !== "",
        )?.sellerDisplayName;
        const date = shippingQuoted
          ? formatDeliveryDate(g.estimatedDeliveryAt)
          : null;
        return (
          <section
            key={g.sellerId}
            data-testid={`checkout-group-${g.sellerId}`}
            className="space-y-3 rounded-xl border border-border-subtle p-3"
          >
            <h3 className="text-sm font-semibold text-text-primary">
              {shopLabel(g.sellerId, shopName)}
            </h3>
            <ul className="divide-y divide-border-subtle">
              {lines.map((it) => (
                <li key={it.id} className="flex items-center gap-3 py-2">
                  <div className="min-w-0 flex-1">
                    <p className="line-clamp-1 text-sm font-medium text-text-primary">
                      {it.title}
                    </p>
                    <p className="text-xs text-text-secondary">
                      {it.variantName ? `Phân loại: ${it.variantName} · ` : ""}x
                      {it.quantity}
                    </p>
                  </div>
                  <PriceTag price={it.unitPrice * it.quantity} size="sm" />
                </li>
              ))}
            </ul>
            <div className="space-y-1">
              <Row label="Tạm tính">{formatPrice(g.itemsSubtotal)}</Row>
              {shippingQuoted ? (
                <Row label="Phí vận chuyển">
                  {g.isFreeShipping ? (
                    <Tag color="success">Freeship</Tag>
                  ) : (
                    formatPrice(g.shippingFee)
                  )}
                </Row>
              ) : null}
              {g.shopDiscountAmount > 0 ? (
                <Row label="Giảm giá shop">
                  {`-${formatPrice(g.shopDiscountAmount)}`}
                </Row>
              ) : null}
              {g.platformDiscountAmount > 0 ? (
                <Row label="Giảm giá sàn">
                  {`-${formatPrice(g.platformDiscountAmount)}`}
                </Row>
              ) : null}
              <Row label="Thành tiền">
                <PriceTag price={g.totalAmount} size="md" />
              </Row>
              {date ? (
                <p className="text-xs text-text-secondary">
                  Nhận hàng dự kiến {date}
                </p>
              ) : null}
            </div>
          </section>
        );
      })}
    </div>
  );
}
