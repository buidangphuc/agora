import NextLink from "next/link";
import type { ReactNode } from "react";

import { Link } from "@/components/ui/Link";
import { CheckoutPendingProvider } from "@/features/order/CheckoutPending";
import { CheckoutStepper } from "@/features/order/CheckoutStepper";
import { isCheckoutEnabled } from "@/lib/flags";

/**
 * Distraction-free checkout shell (the (checkout) route group): minimal header
 * (logo, Stepper, secure hint) and a secure footer, with none of the consumer
 * chrome. The root layout still mounts AnalyticsProvider and ToastProvider
 * around it.
 */
export default async function CheckoutLayout({
  children,
}: {
  children: ReactNode;
}) {
  const checkoutEnabled = await isCheckoutEnabled();
  return (
    <CheckoutPendingProvider>
      <main
        data-checkout-shell=""
        data-testid="checkout-shell"
        className="mx-auto w-full min-w-0 max-w-page flex-1 space-y-6 px-4 py-6 md:space-y-8 md:px-6 md:py-8"
      >
        <header className="rounded-2xl border border-border-subtle bg-surface-card p-4 shadow-card md:p-6">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <NextLink
              href="/"
              className="inline-flex min-h-touch items-center rounded-lg text-xl font-semibold text-action-primary focus-visible:ring-2 focus-visible:ring-focus-ring"
            >
              Marketplace
            </NextLink>
            <p className="rounded-xl bg-surface-muted px-3 py-1 text-xs font-semibold text-text-secondary">
              Thanh toán an toàn
            </p>
          </div>
          <div className="mt-4 md:mt-6">
            <CheckoutStepper checkoutEnabled={checkoutEnabled} />
          </div>
        </header>
        {children}
        <footer className="flex flex-col items-center gap-2 border-t border-border-subtle pt-6 text-center text-xs text-text-secondary">
          <p>Thanh toán an toàn · Môi trường demo, không trừ tiền thật.</p>
          <p className="flex flex-wrap justify-center gap-2">
            <Link href="/account/orders" variant="ghost" size="sm">
              Đơn hàng của tôi
            </Link>
            <Link href="/cart" variant="ghost" size="sm">
              Giỏ hàng
            </Link>
          </p>
        </footer>
      </main>
    </CheckoutPendingProvider>
  );
}
