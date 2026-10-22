import Link from "next/link";

import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Descriptions } from "@/components/ui/Descriptions";
import { Radio } from "@/components/ui/Radio";
import { Tag } from "@/components/ui/Tag";
import { formatPrice } from "@/components/ui/format";
import { AddressSelectorModal } from "@/features/address/AddressSelectorModal";
import { linkButtonPrimary } from "@/features/cart/linkButton";
import type { EcommerceItem } from "@/lib/analytics";
import type { ViewAddress } from "@/lib/gateway/addresses";
import type { ViewCartItem } from "@/lib/gateway/cart";
import type { ViewCheckoutGroup, ViewVoucherQuote } from "@/lib/gateway/orders";
import type { ViewVoucher } from "@/lib/gateway/promotion";
import { CheckoutGroups } from "./CheckoutGroups";
import { PaymentOptionsGrid } from "./PaymentOptionsGrid";
import { PlaceOrderForm } from "./PlaceOrderForm";
import { VoucherModal } from "./VoucherModal";
import { type CheckoutState, checkoutHref } from "./checkoutParams";
import { paymentOptionFor } from "./paymentOptions";
import type { ShippingFee } from "./shippingCopy";

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
  groups,
  items,
}: {
  shipping: ShippingFee;
  state: CheckoutState;
  groups: ViewCheckoutGroup[];
  items: ViewCartItem[];
}) {
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Phương thức vận chuyển</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            <Radio
              name="shippingMethod"
              value="standard"
              checked
              readOnly
              label="Giao hàng tiêu chuẩn"
              description="Phí vận chuyển ước tính theo địa chỉ nhận hàng."
            />
            <CheckoutGroups groups={groups} items={items} shippingQuoted />
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
  groups,
  quotes,
  available,
}: {
  state: CheckoutState;
  groups: ViewCheckoutGroup[];
  quotes: ViewVoucherQuote[];
  available?: Record<string, ViewVoucher[]>;
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
        state={state}
        groups={groups}
        quotes={quotes}
        available={available}
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
  groups,
  shippingQuoted,
  shipping,
  total,
  trackItems,
  vouchers,
}: {
  state: CheckoutState;
  address: Pick<
    ViewAddress,
    "id" | "recipientName" | "phone" | "street" | "ward" | "district" | "city"
  >;
  items: ViewCartItem[];
  groups: ViewCheckoutGroup[];
  shippingQuoted: boolean;
  shipping: ShippingFee;
  total: number;
  trackItems: EcommerceItem[];
  vouchers: ViewVoucherQuote[];
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
                children: [
                  address.street,
                  address.ward,
                  address.district,
                  address.city,
                ]
                  .filter(Boolean)
                  .join(", "),
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
                children: state.pv ?? "-",
              },
            ]}
          />
          <CheckoutGroups
            groups={groups}
            items={items}
            shippingQuoted={shippingQuoted}
          />
        </CardContent>
      </Card>
      <PlaceOrderForm
        addressId={address.id}
        method={state.pay}
        vouchers={vouchers}
        itemIds={items.map((it) => it.id)}
        confirmedAddress={{
          recipientName: address.recipientName,
          phone: address.phone,
          street: address.street,
          ward: address.ward,
          district: address.district,
          city: address.city,
        }}
        total={total}
        shippingTier={shipping.isFree ? "FREE" : "STANDARD"}
        items={trackItems}
        backHref={checkoutHref({ ...state, step: "payment" })}
      />
    </div>
  );
}
