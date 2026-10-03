"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { PriceTag } from "@/components/ui/PriceTag";
import { useToast } from "@/components/ui/ToastProvider";
import { addToCartAction } from "@/features/cart/actions";
import type { ViewVariant } from "@/lib/gateway/listings";

export function VariantSelector({
  listingId,
  basePrice,
  currency = "VND",
  baseStock,
  variants = [],
}: {
  listingId: string;
  basePrice: number;
  currency?: string;
  baseStock: number;
  variants?: ViewVariant[];
}) {
  const router = useRouter();
  const toast = useToast();
  const [selectedVariantId, setSelectedVariantId] = useState<string>(
    variants.length > 0 ? variants[0].id : "",
  );
  const [quantity, setQuantity] = useState(1);
  const [adding, setAdding] = useState(false);
  const [feedback, setFeedback] = useState("");

  const selectedVariant = variants.find((v) => v.id === selectedVariantId);
  const currentPrice =
    selectedVariant && selectedVariant.price > 0
      ? selectedVariant.price
      : basePrice;
  const currentStock =
    variants.length > 0
      ? selectedVariant
        ? selectedVariant.stock
        : 0
      : baseStock;
  const isOutOfStock = currentStock <= 0;

  function handleSelect(id: string) {
    setSelectedVariantId(id);
    setQuantity(1);
  }

  function handleDecrease() {
    setQuantity((q) => Math.max(1, q - 1));
  }

  function handleIncrease() {
    setQuantity((q) => Math.min(currentStock, q + 1));
  }

  async function handleAddToCart(redirectCheckout = false) {
    setAdding(true);
    setFeedback("");
    try {
      const res = await addToCartAction(listingId, selectedVariantId, quantity);
      if (res.ok) {
        if (redirectCheckout) {
          router.push("/checkout");
        } else {
          const successMsg = "✓ Đã thêm vào giỏ hàng thành công!";
          setFeedback(successMsg);
          toast.success(successMsg);
          setTimeout(() => setFeedback(""), 3000);
        }
      } else {
        const errorMsg = res.message || "Không thể thêm vào giỏ hàng.";
        setFeedback(errorMsg);
        toast.error(errorMsg);
      }
    } catch {
      const errorMsg = "Có lỗi xảy ra khi thêm vào giỏ hàng.";
      setFeedback(errorMsg);
      toast.error(errorMsg);
    } finally {
      setAdding(false);
    }
  }

  return (
    <Card className="mt-6 space-y-5 rounded-2xl border-gray-200/80 bg-gray-50/50 p-6 shadow-preline-card">
      {/* Current Price */}
      <div className="flex items-baseline gap-3 flex-wrap">
        <PriceTag price={currentPrice} size="xl" />
        {selectedVariant?.sku && (
          <span className="text-xs text-gray-400">
            SKU: #{selectedVariant.sku}
          </span>
        )}
      </div>

      {/* Variant Selection Chips */}
      {variants.length > 0 && (
        <div>
          <span className="mb-2.5 block text-xs font-semibold uppercase tracking-wider text-gray-600">
            Tùy chọn / Phân loại
          </span>
          <div className="flex flex-wrap gap-2.5">
            {variants.map((v) => {
              const isSelected = v.id === selectedVariantId;
              const out = v.stock <= 0;
              return (
                <button
                  key={v.id}
                  type="button"
                  onClick={() => handleSelect(v.id)}
                  className={`relative rounded-xl border px-4 py-2 text-xs font-medium transition cursor-pointer select-none ${
                    isSelected
                      ? "border-primary-500 bg-primary-50/80 text-primary-600 ring-2 ring-primary-100 font-semibold shadow-xs"
                      : "border-gray-200 bg-white text-gray-700 hover:border-gray-300 hover:bg-gray-50"
                  } ${out ? "opacity-50 cursor-not-allowed" : ""}`}
                >
                  <span>{v.name}</span>
                  {out && (
                    <span className="ml-1.5 rounded bg-gray-100 px-1 py-0.2 text-[10px] text-gray-500">
                      Hết hàng
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        </div>
      )}

      {/* Stock status & Quantity selector */}
      <div className="flex items-center justify-between border-t border-gray-200/80 pt-4 flex-wrap gap-3">
        <div>
          <span className="block text-xs text-gray-500">Tình trạng kho:</span>
          {isOutOfStock ? (
            <span className="text-xs font-semibold text-rose-600 flex items-center gap-1 mt-0.5">
              <span>●</span> Tạm hết hàng
            </span>
          ) : (
            <span className="text-xs font-semibold text-emerald-600 flex items-center gap-1 mt-0.5">
              <span>●</span> Còn {currentStock} sản phẩm sẵn có
            </span>
          )}
        </div>

        <div className="flex items-center gap-2.5">
          <span className="text-xs text-gray-500">Số lượng:</span>
          <div className="flex items-center rounded-lg border border-gray-200 bg-white shadow-2xs overflow-hidden">
            <button
              type="button"
              onClick={handleDecrease}
              disabled={isOutOfStock || quantity <= 1 || adding}
              className="px-3 py-1.5 text-xs font-semibold text-gray-600 hover:bg-gray-100 disabled:opacity-40 transition cursor-pointer"
            >
              -
            </button>
            <span className="w-8 text-center text-xs font-bold text-gray-900">
              {quantity}
            </span>
            <button
              type="button"
              onClick={handleIncrease}
              disabled={isOutOfStock || quantity >= currentStock || adding}
              className="px-3 py-1.5 text-xs font-semibold text-gray-600 hover:bg-gray-100 disabled:opacity-40 transition cursor-pointer"
            >
              +
            </button>
          </div>
        </div>
      </div>

      {feedback && (
        <p
          className={`text-xs font-medium ${
            feedback.startsWith("✓") ? "text-emerald-600" : "text-rose-600"
          }`}
        >
          {feedback}
        </p>
      )}

      {/* Action CTA Buttons */}
      <div className="flex gap-3 pt-2">
        <Button
          type="button"
          variant="outline"
          size="lg"
          disabled={isOutOfStock || adding}
          isLoading={adding}
          onClick={() => handleAddToCart(false)}
          className="flex-1 font-bold text-primary-600 border-primary-500 hover:bg-primary-50/50"
        >
          Thêm vào giỏ
        </Button>
        <Button
          type="button"
          variant="primary"
          size="lg"
          disabled={isOutOfStock || adding}
          onClick={() => handleAddToCart(true)}
          className="flex-1 font-bold shadow-md"
        >
          {isOutOfStock ? "Tạm hết hàng" : "Mua ngay"}
        </Button>
      </div>
    </Card>
  );
}
