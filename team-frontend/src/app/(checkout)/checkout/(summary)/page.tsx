import Link from "next/link";
import { redirect } from "next/navigation";

import { Button } from "@/components/ui/Button";
import { Result } from "@/components/ui/Result";
import { linkButtonPrimary } from "@/features/cart/linkButton";
import { BeginCheckoutBeacon } from "@/features/order/BeginCheckoutBeacon";
import {
  AddressStep,
  ConfirmStep,
  PaymentStep,
  ShippingStep,
} from "@/features/order/CheckoutSteps";
import { OrderSummary } from "@/features/order/OrderSummary";
import { MobilePlaceOrderButton } from "@/features/order/PlaceOrderForm";
import {
  type RawSearchParams,
  checkoutHref,
  resolveCheckout,
} from "@/features/order/checkoutParams";
import { orderTotal } from "@/features/order/orderTotal";
import { computeShippingFee } from "@/features/order/shipping";
import { isCheckoutEnabled } from "@/lib/flags";
import { listAddresses } from "@/lib/gateway/addresses";
import { getCart } from "@/lib/gateway/cart";
import { listVouchers, previewVoucher } from "@/lib/gateway/promotion";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function CheckoutPage({
  searchParams,
}: {
  searchParams: RawSearchParams;
}) {
  const me = getPrincipal();
  if (!me) redirect("/login");

  // Kill-switch: evaluated server-side. When off, render the unavailable notice
  // instead of the wizard (covers direct navigation to /checkout). The
  // authoritative block still lives in team-order's CreateOrder.
  const checkoutEnabled = await isCheckoutEnabled();
  if (!checkoutEnabled) {
    return (
      <Result
        status="info"
        title="Thanh toán tạm thời không khả dụng"
        subTitle="Chức năng thanh toán đang tạm dừng. Vui lòng thử lại sau ít phút."
        extra={
          <Link href="/cart" className={linkButtonPrimary}>
            Quay lại giỏ hàng
          </Link>
        }
      />
    );
  }

  const [cart, addresses] = await Promise.all([getCart(), listAddresses()]);
  if (cart.items.length === 0) redirect("/cart");

  const state = resolveCheckout(searchParams, addresses);
  if (state.needsRedirect) redirect(checkoutHref(state));

  const address = addresses.find((a) => a.id === state.addr);
  // One voucher per order, validated against the first shop's items (unchanged
  // contract). The discount is the server's preview, never computed here.
  const sellerId = cart.items[0]?.sellerId ?? "";
  const code = state.voucher?.trim();
  const [preview, vouchers] = await Promise.all([
    code ? previewVoucher(code, cart.subtotal, sellerId) : null,
    state.step === "payment" ? listVouchers(sellerId) : Promise.resolve([]),
  ]);
  const applied = preview?.valid ? code : undefined;
  const discount = preview?.valid ? preview.discountAmount : 0;
  const view = { ...state, voucher: applied };

  const shipping = computeShippingFee(address?.city ?? "", cart.subtotal);
  const total = orderTotal(cart.subtotal, discount, shipping.fee);
  const trackItems = cart.items.map((it, idx) => ({
    itemId: it.listingId,
    itemName: it.title,
    price: it.unitPrice,
    quantity: it.quantity,
    index: idx + 1,
  }));

  const nextHref =
    state.step === "address"
      ? view.addr
        ? checkoutHref({ ...view, step: "shipping" })
        : undefined
      : state.step === "shipping"
        ? checkoutHref({ ...view, step: "payment" })
        : checkoutHref({ ...view, step: "confirm" });
  const mobileAction =
    state.step === "confirm" ? (
      <MobilePlaceOrderButton />
    ) : nextHref ? (
      <Link href={nextHref} className={linkButtonPrimary}>
        Tiếp tục
      </Link>
    ) : (
      <Button size="lg" disabled>
        Tiếp tục
      </Button>
    );

  const steps = {
    address: <AddressStep addresses={addresses} state={view} />,
    shipping: <ShippingStep shipping={shipping} state={view} />,
    payment: (
      <PaymentStep
        state={view}
        vouchers={vouchers}
        subtotal={cart.subtotal}
        sellerId={sellerId}
        appliedDiscount={discount}
      />
    ),
    confirm: address ? (
      <ConfirmStep
        state={view}
        address={address}
        items={cart.items}
        shipping={shipping}
        total={total}
        trackItems={trackItems}
      />
    ) : null,
  } as const;

  return (
    <section className="py-2 pb-24 lg:pb-2">
      <BeginCheckoutBeacon value={cart.subtotal} items={trackItems} />
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div className="lg:col-span-2">{steps[state.step]}</div>
        <div>
          <OrderSummary
            itemCount={cart.items.length}
            subtotal={cart.subtotal}
            discount={discount}
            voucherCode={applied}
            shipping={address ? shipping : null}
            action={
              <Link
                href="/cart"
                className="block text-center text-xs text-text-secondary hover:text-action-primary"
              >
                Quay lại giỏ hàng
              </Link>
            }
            mobileAction={mobileAction}
          />
        </div>
      </div>
    </section>
  );
}
