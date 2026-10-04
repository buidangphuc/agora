import Link from "next/link";

import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Descriptions } from "@/components/ui/Descriptions";
import { Image } from "@/components/ui/Image";
import { PriceTag } from "@/components/ui/PriceTag";
import { Radio } from "@/components/ui/Radio";
import { Tag } from "@/components/ui/Tag";
import { formatPrice } from "@/components/ui/format";
import { AddressSelectorModal } from "@/features/address/AddressSelectorModal";
import { formatAddress } from "@/features/address/formatAddress";
import { linkButtonPrimary } from "@/features/cart/linkButton";
import { VoucherModal } from "@/features/voucher/VoucherModal";
import type { EcommerceItem } from "@/lib/analytics";
import type { ViewAddress } from "@/lib/gateway/addresses";
import type { ViewCartItem } from "@/lib/gateway/cart";
import type { ViewVoucher } from "@/lib/gateway/promotion";
import { getImageUrl } from "@/lib/media";
import { PaymentOptionsGrid } from "./PaymentOptionsGrid";
import { PlaceOrderForm } from "./PlaceOrderForm";
import { type CheckoutState, checkoutHref } from "./checkoutParams";
import { paymentOptionFor } from "./paymentOptions";
import type { ShippingFee } from "./shipping";

function StepActions({
  backHref,
  nextHref,
}: {
  backHref?: string;
  /** Omit to render a disabled "Tiếp tục". */
  nextHref?: string;
}) {
  return (
    <div className="flex items-center justify-between gap-3">
      {backHref ? (
        <Link
          href={backHref}
          className="rounded-xs text-sm font-medium text-text-secondary hover:text-action-primary"
        >
          Quay lại
        </Link>
      ) : (
        <span />
      )}
      {nextHref ? (
        <Link href={nextHref} className={`${linkButtonPrimary} min-w-40`}>
          Tiếp tục
        </Link>
      ) : (
        <Button size="lg" disabled className="min-w-40">
          Tiếp tục
        </Button>
      )}
    </div>
  );
}

export function AddressStep({
  addresses,
  state,
}: {
  addresses: ViewAddress[];
  state: CheckoutState;
}) {
  return (
    <div className="space-y-4">
      <AddressSelectorModal addresses={addresses} selectedId={state.addr} />
      <StepActions
        nextHref={
          state.addr ? checkoutHref({ ...state, step: "shipping" }) : undefined
        }
      />
    </div>
  );
}

export function ShippingStep({
  shipping,
  state,
}: {
  shipping: ShippingFee;
  state: CheckoutState;
}) {
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Phương thức vận chuyển</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between gap-3 rounded-xl border border-border-strong bg-surface-muted p-3.5 ring-2 ring-focus-ring">
            <Radio
              name="shippingMethod"
              value="standard"
              checked
              readOnly
              label="Giao hàng tiêu chuẩn"
              description="Phí vận chuyển ước tính theo địa chỉ nhận hàng."
            />
            {shipping.isFree ? (
              <Tag color="success">Freeship</Tag>
            ) : (
              <PriceTag price={shipping.fee} size="md" />
            )}
          </div>
        </CardContent>
      </Card>
      <StepActions
        backHref={checkoutHref({ ...state, step: "address" })}
        nextHref={checkoutHref({ ...state, step: "payment" })}
      />
    </div>
  );
}

export function PaymentStep({
  state,
  vouchers,
  subtotal,
  sellerId,
  appliedDiscount,
}: {
  state: CheckoutState;
  vouchers: ViewVoucher[];
  subtotal: number;
  sellerId: string;
  appliedDiscount: number;
}) {
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Phương thức thanh toán</CardTitle>
        </CardHeader>
        <CardContent>
          <PaymentOptionsGrid selected={state.pay} />
        </CardContent>
      </Card>
      <VoucherModal
        vouchers={vouchers}
        subtotal={subtotal}
        sellerId={sellerId}
        appliedCode={state.voucher}
        appliedDiscount={appliedDiscount}
      />
      <StepActions
        backHref={checkoutHref({ ...state, step: "shipping" })}
        nextHref={checkoutHref({ ...state, step: "confirm" })}
      />
    </div>
  );
}

export function ConfirmStep({
  state,
  address,
  items,
  shipping,
  total,
  trackItems,
}: {
  state: CheckoutState;
  address: ViewAddress;
  items: ViewCartItem[];
  shipping: ShippingFee;
  total: number;
  trackItems: EcommerceItem[];
}) {
  const payment = paymentOptionFor(state.pay);
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Xác nhận đơn hàng</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <Descriptions
            column={1}
            items={[
              {
                key: "recipient",
                label: "Người nhận",
                children: `${address.recipientName} · ${address.phone}`,
              },
              {
                key: "address",
                label: "Địa chỉ",
                children: formatAddress(address),
              },
              {
                key: "shipping",
                label: "Vận chuyển",
                children: (
                  <span className="inline-flex flex-wrap items-center gap-2">
                    Giao hàng tiêu chuẩn
                    {shipping.isFree ? (
                      <Tag color="success">Freeship</Tag>
                    ) : (
                      formatPrice(shipping.fee)
                    )}
                  </span>
                ),
              },
              {
                key: "payment",
                label: "Thanh toán",
                children: payment.title,
              },
              {
                key: "voucher",
                label: "Mã giảm giá",
                children: state.voucher ?? "-",
              },
            ]}
          />
          <ul className="divide-y divide-border-subtle rounded-xl border border-border-subtle">
            {items.map((it) => (
              <li key={it.id} className="flex items-center gap-3 p-3">
                <div className="w-16 shrink-0">
                  <Image
                    src={getImageUrl(it.imageUrl)}
                    alt={it.title}
                    aspect="square"
                    loading="lazy"
                  />
                </div>
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
        </CardContent>
      </Card>
      <PlaceOrderForm
        addressId={address.id}
        method={state.pay}
        voucherCode={state.voucher}
        total={total}
        shippingTier={shipping.isFree ? "FREE" : "STANDARD"}
        items={trackItems}
        backHref={checkoutHref({ ...state, step: "payment" })}
      />
    </div>
  );
}
