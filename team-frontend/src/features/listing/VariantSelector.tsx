"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { addToCartAction } from "@/features/cart/actions";
import type { ViewVariant } from "@/lib/gateway/listings";

export function VariantSelector({
  listingId,
  basePrice,
  currency,
  baseStock,
  variants = [],
}: {
  listingId: string;
  basePrice: number;
  currency: string;
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
        const errorMsg = res.message || "Thêm vào giỏ thất bại.";
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
    <div className="mt-6 space-y-5 rounded-lg border bg-gray-50/50 p-5">
      {/* Current Price */}
      <div className="flex items-baseline gap-3">
        <span className="text-2xl font-bold text-brand">
          {currentPrice.toLocaleString("vi-VN")} {currency}
        </span>
        {selectedVariant?.sku && (
          <span className="text-xs text-gray-400">
            SKU: {selectedVariant.sku}
          </span>
        )}
      </div>

      {/* Variant Selection Chips */}
      {variants.length > 0 && (
        <div>
          <span className="mb-2 block text-xs font-semibold uppercase tracking-wider text-gray-500">
            Tùy chọn / Phân loại
          </span>
          <div className="flex flex-wrap gap-2">
            {variants.map((v) => {
              const isSelected = v.id === selectedVariantId;
              const out = v.stock <= 0;
              return (
                <button
                  key={v.id}
                  type="button"
                  onClick={() => handleSelect(v.id)}
                  className={`relative rounded-md border px-3.5 py-2 text-sm font-medium transition ${
                    isSelected
                      ? "border-brand bg-orange-50 text-brand shadow-xs"
                      : "border-gray-200 bg-white text-gray-700 hover:border-gray-300"
                  } ${out ? "opacity-60" : ""}`}
                >
                  <span>{v.name}</span>
                  {out && (
                    <span className="ml-1.5 rounded bg-gray-100 px-1 py-0.5 text-[10px] text-gray-500">
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
      <div className="flex items-center justify-between border-t pt-4">
        <div>
          <span className="block text-xs text-gray-500">Tình trạng kho:</span>
          {isOutOfStock ? (
            <span className="text-sm font-semibold text-red-600">
              🔴 Tạm hết hàng
            </span>
          ) : (
            <span className="text-sm font-medium text-emerald-700">
              🟢 Còn {currentStock} sản phẩm
            </span>
          )}
        </div>

        <div className="flex items-center gap-2">
          <span className="text-xs text-gray-500">Số lượng:</span>
          <div className="flex items-center rounded-md border bg-white">
            <button
              type="button"
              onClick={handleDecrease}
              disabled={isOutOfStock || quantity <= 1 || adding}
              className="px-2.5 py-1 text-sm font-semibold text-gray-600 hover:bg-gray-100 disabled:opacity-40"
            >
              -
            </button>
            <span className="w-8 text-center text-sm font-medium">
              {quantity}
            </span>
            <button
              type="button"
              onClick={handleIncrease}
              disabled={isOutOfStock || quantity >= currentStock || adding}
              className="px-2.5 py-1 text-sm font-semibold text-gray-600 hover:bg-gray-100 disabled:opacity-40"
            >
              +
            </button>
          </div>
        </div>
      </div>

      {feedback && (
        <p
          className={`text-xs font-medium ${
            feedback.startsWith("✓") ? "text-green-600" : "text-red-600"
          }`}
        >
          {feedback}
        </p>
      )}

      {/* Action CTA Buttons */}
      <div className="flex gap-3 pt-2">
        <button
          type="button"
          disabled={isOutOfStock || adding}
          onClick={() => handleAddToCart(false)}
          className="flex-1 rounded-md border border-brand bg-orange-50/50 py-2.5 font-medium text-brand transition hover:bg-orange-100/50 disabled:cursor-not-allowed disabled:border-gray-200 disabled:bg-gray-100 disabled:text-gray-400"
        >
          {adding ? "Đang thêm..." : "Thêm vào giỏ"}
        </button>
        <button
          type="button"
          disabled={isOutOfStock || adding}
          onClick={() => handleAddToCart(true)}
          className="flex-1 rounded-md bg-brand py-2.5 font-medium text-white shadow-sm transition hover:bg-brand-dark disabled:cursor-not-allowed disabled:bg-gray-300"
        >
          {isOutOfStock ? "Tạm hết hàng" : "Mua ngay"}
        </button>
      </div>
    </div>
  );
}
