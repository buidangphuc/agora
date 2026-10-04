"use client";

import { useSearchParams } from "next/navigation";

import { Stepper } from "@/components/ui/Stepper";
import { useCheckoutPending } from "./CheckoutPending";
import {
  CHECKOUT_STEPS,
  type CheckoutStep,
  checkoutHref,
  stepIndex,
} from "./checkoutParams";

export const STEP_TITLES: Record<CheckoutStep, string> = {
  address: "Địa chỉ",
  shipping: "Vận chuyển",
  payment: "Thanh toán",
  confirm: "Xác nhận",
};

/**
 * The four-step Stepper. The current step is read from `?step=` (the page has
 * already validated it); completed steps link back with the selections kept.
 * Navigation is inert while an order is being placed.
 */
export function CheckoutStepper() {
  const params = useSearchParams();
  const { pending } = useCheckoutPending();
  const raw = params.get("step");
  const current: CheckoutStep = CHECKOUT_STEPS.includes(raw as CheckoutStep)
    ? (raw as CheckoutStep)
    : "address";
  const currentIndex = stepIndex(current);
  const payRaw = params.get("pay");

  return (
    <div className="w-full">
      <Stepper
        steps={CHECKOUT_STEPS.map((step, i) => ({
          id: step,
          title: STEP_TITLES[step],
          status:
            i < currentIndex
              ? "complete"
              : i === currentIndex
                ? "current"
                : "upcoming",
          ...(i < currentIndex
            ? {
                href: checkoutHref({
                  step,
                  addr: params.get("addr") ?? undefined,
                  pay: payRaw === null ? undefined : Number(payRaw),
                  voucher: params.get("voucher") ?? undefined,
                }),
                disabled: pending,
              }
            : {}),
        }))}
      />
      <p className="mt-2 text-xs text-text-secondary sm:hidden">
        Bước {currentIndex + 1}/{CHECKOUT_STEPS.length}: {STEP_TITLES[current]}
      </p>
    </div>
  );
}
