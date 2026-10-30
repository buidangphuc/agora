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
        className="mx-auto w-full max-w-page flex-1 space-y-6 px-4 py-5"
      >
        <header className="rounded-xl border border-border-subtle bg-surface-card p-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <NextLink
              href="/"
              className="text-xl font-semibold text-action-primary"
            >
              Marketplace
            </NextLink>
            <p className="text-xs text-text-secondary">Thanh toán an toàn</p>
          </div>
          <div className="mt-4">
            <CheckoutStepper checkoutEnabled={checkoutEnabled} />
          </div>
        </header>
        {children}
        <footer className="border-t border-border-subtle pt-4 text-center text-xs text-text-secondary">
          <p>Thanh toán an toàn · Môi trường demo, không trừ tiền thật.</p>
          <p className="mt-1 space-x-3">
            <Link href="/account/orders" variant="subtle">
              Đơn hàng của tôi
            </Link>
            <Link href="/cart" variant="subtle">
              Giỏ hàng
            </Link>
          </p>
        </footer>
      </main>
    </CheckoutPendingProvider>
  );
}
