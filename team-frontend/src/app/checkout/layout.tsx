import Link from "next/link";
import type { ReactNode } from "react";

import { CheckoutPendingProvider } from "@/features/order/CheckoutPending";
import { CheckoutStepper } from "@/features/order/CheckoutStepper";

/**
 * Distraction-free checkout shell: minimal header (logo, Stepper, secure hint)
 * and a secure footer. The root layout (shared, owned elsewhere) still mounts
 * AnalyticsProvider and ToastProvider around this segment; its visible chrome
 * (mega search header, disclaimer banner, big footer) is hidden for this
 * segment only. The rule lives in this layout, so it is dropped with the
 * segment on navigation and /cart keeps the consumer shell. The CSS text avoids
 * `>`, quotes and `&`: React escapes them in a style text child, which would
 * break hydration.
 */
const HIDE_CONSUMER_CHROME =
  "body header:not([data-checkout-shell] header), body footer:not([data-checkout-shell] footer), body div.bg-amber-50 { display: none; }";

export default function CheckoutLayout({ children }: { children: ReactNode }) {
  return (
    <CheckoutPendingProvider>
      <style>{HIDE_CONSUMER_CHROME}</style>
      <div
        data-checkout-shell=""
        data-testid="checkout-shell"
        className="space-y-6"
      >
        <header className="rounded-xl border border-border-subtle bg-surface-card p-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Link
              href="/"
              className="text-xl font-semibold text-action-primary"
            >
              Marketplace
            </Link>
            <p className="text-xs text-text-secondary">Thanh toán an toàn</p>
          </div>
          <div className="mt-4">
            <CheckoutStepper />
          </div>
        </header>
        {children}
        <footer className="border-t border-border-subtle pt-4 text-center text-xs text-text-secondary">
          <p>Thanh toán an toàn · Môi trường demo, không trừ tiền thật.</p>
          <p className="mt-1 space-x-3">
            <Link href="/account/orders" className="hover:text-action-primary">
              Đơn hàng của tôi
            </Link>
            <Link href="/cart" className="hover:text-action-primary">
              Giỏ hàng
            </Link>
          </p>
        </footer>
      </div>
    </CheckoutPendingProvider>
  );
}
