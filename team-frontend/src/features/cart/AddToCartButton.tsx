"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import type { ViewListing } from "@/lib/gateway/listings";
import { track } from "@/lib/track";
import { addToCartAction } from "./actions";

export function AddToCartButton({
  listing,
  selectedVariantId,
}: {
  listing: ViewListing;
  selectedVariantId?: string;
}) {
  const router = useRouter();
  const { success, error } = useToast();
  const [quantity, setQuantity] = useState(1);
  const [loading, setLoading] = useState(false);

  async function handleAdd(redirectNow = false) {
    setLoading(true);
    try {
      const res = await addToCartAction(
        listing.id,
        selectedVariantId || listing.variants?.[0]?.id,
        quantity,
      );

      if (res.ok) {
        track({
          type: "add_to_cart",
          listingId: listing.id,
          properties: { quantity: String(quantity) },
        });
        success(`✓ Đã thêm ${quantity} sản phẩm vào giỏ hàng!`);
        if (redirectNow) {
          router.push("/cart");
        }
      } else {
        error(res.message || "Thêm vào giỏ hàng thất bại.");
      }
    } catch {
      error("Có lỗi xảy ra khi thêm vào giỏ hàng.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="space-y-4">
      {/* Quantity Selector */}
      <div className="flex items-center gap-4 text-xs text-gray-600">
        <span className="w-24 text-gray-500">Số Lượng:</span>
        <div className="flex items-center">
          <button
            type="button"
            onClick={() => setQuantity(Math.max(1, quantity - 1))}
            className="grid h-8 w-8 place-items-center border border-gray-300 bg-white text-gray-600 hover:bg-gray-50"
          >
            -
          </button>
          <input
            type="number"
            value={quantity}
            onChange={(e) =>
              setQuantity(
                Math.max(
                  1,
                  Math.min(listing.stock || 100, Number(e.target.value) || 1),
                ),
              )
            }
            className="h-8 w-14 border-y border-gray-300 text-center text-xs font-semibold text-gray-800 outline-none"
          />
          <button
            type="button"
            onClick={() =>
              setQuantity(Math.min(listing.stock || 100, quantity + 1))
            }
            className="grid h-8 w-8 place-items-center border border-gray-300 bg-white text-gray-600 hover:bg-gray-50"
          >
            +
          </button>
        </div>
        <span className="text-gray-400 text-[11px]">
          {listing.stock} sản phẩm có sẵn
        </span>
      </div>

      {/* Buttons: Add to Cart & Buy Now */}
      <div className="flex flex-wrap items-center gap-3 pt-2">
        <button
          type="button"
          disabled={loading || listing.stock <= 0}
          onClick={() => handleAdd(false)}
          className="flex items-center justify-center gap-2 rounded-xs border border-brand bg-[#ffeee8] px-6 py-3 text-xs font-bold text-brand hover:bg-[#ffe5dc] transition shadow-xs disabled:opacity-50"
        >
          <span>🛒</span>
          <span>{loading ? "Đang thêm..." : "Thêm Vào Giỏ Hàng"}</span>
        </button>

        <button
          type="button"
          disabled={loading || listing.stock <= 0}
          onClick={() => handleAdd(true)}
          className="flex items-center justify-center rounded-xs bg-brand px-10 py-3 text-xs font-bold text-white shadow-xs hover:bg-brand-dark transition disabled:opacity-50"
        >
          Mua Ngay
        </button>
      </div>
    </div>
  );
}
