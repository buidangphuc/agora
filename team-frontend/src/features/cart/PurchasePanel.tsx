"use client";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { QuantityPicker } from "@/components/ui/QuantityPicker";
import { usePurchase } from "./PurchaseContext";

/**
 * Purchase actions of the product page: quantity (1..stock), the stock line and
 * `Thêm vào giỏ` / `Mua ngay`. Pending state, the selected variant and the
 * quantity live in the PurchaseProvider so the mobile BuyBar shares them. The
 * inline action row is hidden below `lg`; the BuyBar shows the one visible set
 * there.
 */
export function PurchasePanel() {
  const { listing, selected, quantity, setQuantity, pending, add } =
    usePurchase();
  const outOfStock = selected.stock <= 0;
  const busy = pending !== null;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-sm text-text-secondary">Số lượng</span>
        <QuantityPicker
          value={quantity}
          onChange={setQuantity}
          min={1}
          max={Math.max(1, selected.stock)}
          disabled={outOfStock || busy}
        />
        <span data-testid="pdp-stock" className="text-xs text-text-secondary">
          {outOfStock ? "Hết hàng" : `${selected.stock} sản phẩm có sẵn`}
        </span>
      </div>

      {outOfStock && (
        <Alert
          type="warning"
          title={
            listing.variants.length > 0
              ? "Phân loại này đã hết hàng"
              : "Sản phẩm này đã hết hàng"
          }
          description={
            listing.variants.length > 0
              ? "Vui lòng chọn phân loại khác."
              : undefined
          }
        />
      )}

      <div className="hidden gap-3 lg:flex">
        <Button
          variant="outline"
          size="lg"
          data-testid="pdp-add-to-cart"
          disabled={outOfStock || busy}
          isLoading={pending === "add"}
          onClick={() => add(false)}
          className="border-action-primary text-action-primary"
        >
          Thêm vào giỏ
        </Button>
        <Button
          variant="primary"
          size="lg"
          data-testid="pdp-buy-now"
          disabled={outOfStock || busy}
          isLoading={pending === "buy"}
          onClick={() => add(true)}
        >
          Mua ngay
        </Button>
      </div>
    </div>
  );
}
