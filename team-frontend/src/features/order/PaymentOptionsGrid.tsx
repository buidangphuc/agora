"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";

import { Radio } from "@/components/ui/Radio";
import type { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { PAYMENT_OPTIONS } from "./paymentOptions";

/**
 * One Radio card per payment method. Native radios share one name, so arrow
 * keys move the selection; the choice is mirrored to `?pay=` (replace, no
 * history entry) so a reload or a later step keeps it.
 */
export function PaymentOptionsGrid({ selected }: { selected: PaymentMethod }) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const [current, setCurrent] = useState<PaymentMethod>(selected);

  function choose(method: PaymentMethod) {
    setCurrent(method);
    const params = new URLSearchParams(searchParams.toString());
    params.set("pay", String(method));
    router.replace(`${pathname}?${params.toString()}`);
  }

  return (
    <fieldset className="m-0 min-w-0 border-0 p-0">
      <legend className="sr-only">Phương thức thanh toán</legend>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {PAYMENT_OPTIONS.map((opt) => {
          const isSelected = current === opt.method;
          return (
            <div
              key={opt.method}
              data-selected={isSelected ? "true" : "false"}
              className={`flex min-h-12 items-center gap-3 rounded-xl border p-3.5 transition duration-150 ${
                isSelected
                  ? "border-border-strong bg-surface-muted ring-2 ring-focus-ring"
                  : "border-border-subtle hover:border-border-strong"
              }`}
            >
              <span
                aria-hidden="true"
                className="flex h-10 w-12 shrink-0 items-center justify-center rounded-lg bg-surface-page text-xs font-semibold text-text-secondary"
              >
                {opt.slot}
              </span>
              <Radio
                className="flex-1"
                name="paymentMethod"
                value={String(opt.method)}
                checked={isSelected}
                onChange={() => choose(opt.method)}
                label={opt.title}
                description={opt.desc}
              />
            </div>
          );
        })}
      </div>
    </fieldset>
  );
}
