"use client";

import { PriceTag } from "@/components/ui/PriceTag";
import { usePurchase } from "@/features/cart/PurchaseContext";

/**
 * The price of the selected variant (client leaf, so a selection updates it at
 * once; the server renders the same value for the URL's variant). The
 * strike-through and discount badge exist only for a real flash-sale price.
 */
export function SelectedPrice() {
  const { selected, salePrice, displayPrice } = usePurchase();
  return (
    <PriceTag
      price={displayPrice}
      originalPrice={salePrice !== null ? selected.price : undefined}
      size="xl"
    />
  );
}
