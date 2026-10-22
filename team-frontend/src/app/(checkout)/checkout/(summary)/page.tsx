import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { Code, ConnectError } from "@connectrpc/connect";
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
import type { ShippingFee } from "@/features/order/shippingCopy";
import { isCheckoutEnabled } from "@/lib/flags";
import { listAddresses } from "@/lib/gateway/addresses";
import { getCart } from "@/lib/gateway/cart";
import {
  type ViewCheckoutPreview,
  previewCheckout,
} from "@/lib/gateway/orders";
import { listVouchers } from "@/lib/gateway/promotion";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function CheckoutPage(props: {
  searchParams: Promise<RawSearchParams>;
}) {
  const searchParams = await props.searchParams;
  const me = getPrincipal();
  if (!me) redirectGuestToLogin("/checkout", searchParams);

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
  const items =
    state.items.length > 0
      ? cart.items.filter((it) => state.items.includes(it.id))
      : cart.items;
  const sellerId = items[0]?.sellerId ?? "";

  let quote: ViewCheckoutPreview;
  try {
    quote = await previewCheckout({
      addressId: state.addr,
      itemIds: state.items,
      shopVouchers: Object.entries(state.sv).map(([sellerId, code]) => ({
        sellerId,
        code,
      })),
      platformVoucherCode: state.pv,
    });
  } catch (e) {
    if (
      e instanceof ConnectError &&
      e.code === Code.FailedPrecondition &&
      e.rawMessage === "cart is empty"
    ) {
      redirect("/cart");
    }
    throw e;
  }

  const vouchers = state.step === "payment" ? await listVouchers(sellerId) : [];
  const view = state;
  const discount = quote.discountAmount;
  const shipping: ShippingFee | null = quote.shippingQuoted
    ? {
        fee: quote.shippingFee,
        isFree:
          quote.groups.length > 0 &&
          quote.groups.every((g) => g.isFreeShipping),
      }
    : null;
  const total = quote.totalAmount;
  const trackItems = items.map((it, idx) => ({
    itemId: it.listingId,
    itemName: it.title,
    price: it.unitPrice,
    quantity: it.quantity,
    index: idx + 1,
  }));
  const subtotal = quote.itemsSubtotal;

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
    shipping: shipping ? (
      <ShippingStep
        shipping={shipping}
        state={view}
        groups={quote.groups}
        items={items}
      />
    ) : null,
    payment: (
      <PaymentStep
        state={view}
        vouchers={vouchers}
        subtotal={subtotal}
        sellerId={sellerId}
        appliedDiscount={discount}
      />
    ),
    confirm:
      (quote.shippingAddress ?? address) && shipping ? (
        <ConfirmStep
          state={view}
          address={
            (quote.shippingAddress ?? address) as NonNullable<typeof address>
          }
          items={items}
          groups={quote.groups}
          shippingQuoted={quote.shippingQuoted}
          shipping={shipping}
          total={total}
          trackItems={trackItems}
        />
      ) : null,
  } as const;

  return (
    <section className="py-2 pb-24 lg:pb-2">
      <BeginCheckoutBeacon value={subtotal} items={trackItems} />
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div className="lg:col-span-2">{steps[state.step]}</div>
        <div>
          <OrderSummary
            itemCount={items.length}
            subtotal={subtotal}
            discount={discount}
            total={total}
            voucherCode={state.pv}
            shipping={shipping}
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
