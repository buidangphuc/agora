"use client";

import { Button } from "@/components/ui/Button";
import { PriceTag } from "@/components/ui/PriceTag";
import { usePurchase } from "./PurchaseContext";

/**
 * Sticky bottom buy bar for viewports below `lg` (hidden at 1024px and up).
 * It reads the selected variant's price and shares the pending and disabled
 * state with the inline PurchasePanel through the PurchaseProvider. A spacer of
 * the bar's height keeps the end of the page from sitting under it; the bar
 * pads itself by the device safe-area inset.
 */
export function BuyBar() {
  const { selected, displayPrice, pending, add } = usePurchase();
  const outOfStock = selected.stock <= 0;
  const busy = pending !== null;

  return (
    <>
      <div aria-hidden="true" className="h-20 lg:hidden" />
      <div
        data-testid="buy-bar"
        className="fixed inset-x-0 bottom-0 z-20 border-t border-border-subtle bg-surface-card shadow-preline-hover lg:hidden"
        style={{ paddingBottom: "env(safe-area-inset-bottom)" }}
      >
        <div className="mx-auto flex h-20 max-w-page items-center gap-3 px-4">
          <PriceTag price={displayPrice} size="lg" className="shrink-0" />
          <div className="ml-auto flex gap-2">
            <Button
              variant="outline"
              size="md"
              data-testid="bar-add-to-cart"
              disabled={outOfStock || busy}
              isLoading={pending === "add"}
              onClick={() => add(false)}
              className="border-action-primary text-action-primary"
            >
              Thêm vào giỏ
            </Button>
            <Button
              variant="primary"
              size="md"
              data-testid="bar-buy-now"
              disabled={outOfStock || busy}
              isLoading={pending === "buy"}
              onClick={() => add(true)}
            >
              Mua ngay
            </Button>
          </div>
        </div>
      </div>
    </>
  );
}
