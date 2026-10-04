"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/ToastProvider";
import type { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { type EcommerceItem, trackEcommerce } from "@/lib/analytics";
import { useCheckoutPending } from "./CheckoutPending";
import { checkoutAction } from "./actions";

export const PLACE_ORDER_FORM_ID = "place-order-form";

/**
 * "Đặt hàng" in the mobile bottom bar: a second submit button of the same form,
 * so it goes through the same guard; disabled while an order is in flight.
 */
export function MobilePlaceOrderButton() {
  const { pending } = useCheckoutPending();
  return (
    <Button
      type="submit"
      form={PLACE_ORDER_FORM_ID}
      size="lg"
      disabled={pending}
    >
      Đặt hàng
    </Button>
  );
}

export interface PlaceOrderFormProps {
  addressId: string;
  method: PaymentMethod;
  /** Only a server-validated code; omitted when none is applied. */
  voucherCode?: string;
  /** Payable total, for the `purchase` event. */
  total: number;
  shippingTier: "FREE" | "STANDARD";
  items: EcommerceItem[];
  backHref: string;
}

/**
 * The Xác nhận actions. Double-submit protection (mandatory):
 *  - `inFlight` is a ref set synchronously BEFORE the first await, so a second
 *    activation in the same tick (double click, Enter) is a no-op even though
 *    React has not re-rendered the button as disabled yet;
 *  - while pending the button is `isLoading` (disabled, aria-busy, width kept)
 *    and Back / Stepper are inert;
 *  - after `ok:true` the lock is NOT released (no `finally`): the button stays
 *    disabled until the navigation to the payment / orders page completes;
 *  - after `ok:false` or a throw the lock is released and the saga Alert shows.
 */
export function PlaceOrderForm({
  addressId,
  method,
  voucherCode,
  total,
  shippingTier,
  items,
  backHref,
}: PlaceOrderFormProps) {
  const router = useRouter();
  const toast = useToast();
  const { pending, setPending } = useCheckoutPending();
  const inFlight = useRef(false);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const [phase, setPhase] = useState<"idle" | "pending" | "done">("idle");
  const [error, setError] = useState<string | null>(null);

  function release() {
    inFlight.current = false;
    setPhase("idle");
    setPending(false);
  }

  async function placeOrder() {
    if (inFlight.current) return;
    inFlight.current = true;
    setPhase("pending");
    setPending(true);
    setError(null);
    try {
      const res = await checkoutAction(
        addressId,
        undefined,
        method,
        voucherCode,
      );
      if (!res.ok) {
        release();
        setError(res.error || "Đặt hàng thất bại.");
        toast.error(res.error || "Đặt hàng thất bại.");
        return;
      }
      trackEcommerce("purchase", {
        transactionId: res.data?.orderIds[0] || `order-${Date.now()}`,
        currency: "VND",
        value: total,
        coupon: voucherCode,
        shippingTier,
        paymentType: String(method),
        items,
      });
      setPhase("done");
      router.push(res.data?.paymentUrl || "/account/orders?success=1");
    } catch (err: unknown) {
      const message =
        err instanceof Error ? err.message : "Có lỗi xảy ra khi tạo đơn hàng.";
      release();
      setError(message);
      toast.error(message);
    }
  }

  const locked = pending || phase !== "idle";

  return (
    <form
      id={PLACE_ORDER_FORM_ID}
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        void placeOrder();
      }}
    >
      {/* Fixed slot above the actions: the Alert never moves the CTA. */}
      <div data-testid="saga-alert-slot" className="min-h-24">
        {error && (
          <Alert
            type="error"
            title="Không thể đặt hàng"
            description={error}
            action={
              <span className="flex flex-wrap gap-2">
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    setError(null);
                    buttonRef.current?.focus();
                  }}
                >
                  Thử lại
                </Button>
                <Link
                  href="/cart"
                  className="inline-flex items-center justify-center rounded-lg border border-border-strong bg-surface-card px-3 py-1.5 text-xs font-medium text-text-primary shadow-sm hover:bg-surface-muted"
                >
                  Quay lại giỏ hàng
                </Link>
              </span>
            }
          />
        )}
      </div>

      <div className="flex items-center justify-between gap-3">
        {locked ? (
          <span
            aria-disabled="true"
            className="cursor-not-allowed text-sm font-medium text-text-disabled"
          >
            Quay lại
          </span>
        ) : (
          <Link
            href={backHref}
            className="rounded-xs text-sm font-medium text-text-secondary hover:text-action-primary"
          >
            Quay lại
          </Link>
        )}
        <Button
          ref={buttonRef}
          type="submit"
          size="lg"
          className="min-w-40"
          isLoading={phase === "pending"}
          disabled={phase === "done"}
        >
          Đặt hàng
        </Button>
      </div>
    </form>
  );
}
