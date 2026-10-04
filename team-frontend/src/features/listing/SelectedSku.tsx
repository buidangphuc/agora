"use client";

import { usePurchase } from "@/features/cart/PurchaseContext";

/** SKU of the selected variant (client leaf: follows a selection at once). */
export function SelectedSku() {
  const { selected } = usePurchase();
  return <span data-testid="pdp-sku">{selected.sku || "—"}</span>;
}

/** Stock of the selected variant, as a spec value ("12 sản phẩm"). */
export function SelectedStock() {
  const { selected } = usePurchase();
  return <span data-testid="pdp-spec-stock">{selected.stock} sản phẩm</span>;
}
