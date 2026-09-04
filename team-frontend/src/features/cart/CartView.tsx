"use client";

import Link from "next/link";
import { useState } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { formatPrice } from "@/components/ui/format";
import type { ViewCart } from "@/lib/gateway/cart";
import { getImageUrl } from "@/lib/media";
import {
  clearCartAction,
  removeFromCartAction,
  updateCartItemAction,
} from "./actions";

export function CartView({
  initialCart,
  checkoutEnabled = true,
}: {
  initialCart: ViewCart;
  // Resolved server-side from the `checkout-enabled` flag. When false, the
  // checkout entry point is hidden and a notice is shown. Defaults to true
  // (fail-open) so an unmapped caller never blocks checkout.
  checkoutEnabled?: boolean;
}) {
  const [cart, setCart] = useState<ViewCart>(initialCart);
  const [updatingId, setUpdatingId] = useState<string | null>(null);
  const toast = useToast();

  async function handleQuantityChange(itemId: string, newQty: number) {
    if (newQty < 1) return;
    setUpdatingId(itemId);
    try {
      const res = await updateCartItemAction(itemId, newQty);
      if (res.ok) {
        setCart((prev) => {
          const items = prev.items.map((it) =>
            it.id === itemId ? { ...it, quantity: newQty } : it,
          );
          const subtotal = items.reduce(
            (acc, it) => acc + it.unitPrice * it.quantity,
            0,
          );
          return { ...prev, items, subtotal };
        });
      }
    } finally {
      setUpdatingId(null);
    }
  }

  async function handleRemove(itemId: string) {
    setUpdatingId(itemId);
    try {
      const res = await removeFromCartAction(itemId);
      if (res.ok) {
        setCart((prev) => {
          const items = prev.items.filter((it) => it.id !== itemId);
          const subtotal = items.reduce(
            (acc, it) => acc + it.unitPrice * it.quantity,
            0,
          );
          return { ...prev, items, subtotal };
        });
        toast.info("Đã xóa sản phẩm khỏi giỏ hàng.");
      }
    } finally {
      setUpdatingId(null);
    }
  }

  async function handleClear() {
    if (!confirm("Bạn có chắc muốn xóa tất cả sản phẩm trong giỏ?")) return;
    const res = await clearCartAction();
    if (res.ok) {
      setCart({ userId: cart.userId, items: [], subtotal: 0, totalItems: 0 });
      toast.info("Đã làm trống giỏ hàng.");
    }
  }

  if (cart.items.length === 0) {
    return (
      <div className="rounded-xs border border-gray-100 bg-white p-12 text-center shadow-shopee">
        <p className="text-5xl">🛒</p>
        <h2 className="mt-4 text-base font-bold text-gray-800">
          Giỏ hàng của bạn đang trống
        </h2>
        <p className="mt-1 text-xs text-gray-500">
          Hãy khám phá hàng triệu sản phẩm nổi bật và săn sale ngay hôm nay!
        </p>
        <Link
          href="/"
          className="mt-6 inline-block rounded-xs bg-brand px-8 py-2.5 text-xs font-bold text-white shadow-md hover:bg-brand-dark uppercase tracking-wider transition"
        >
          Mua Sắm Ngay
        </Link>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
      {/* Items list */}
      <div className="space-y-3 lg:col-span-2">
        <div className="flex items-center justify-between border-b border-gray-200 bg-white p-4 rounded-xs shadow-shopee">
          <h1 className="text-sm font-bold text-gray-900 uppercase tracking-wide">
            Giỏ Hàng ({cart.items.length} sản phẩm)
          </h1>
          <button
            type="button"
            onClick={handleClear}
            className="text-xs text-gray-400 hover:text-red-600 transition"
          >
            Xóa tất cả
          </button>
        </div>

        <div className="space-y-2">
          {cart.items.map((it) => (
            <div
              key={it.id}
              className="flex items-center gap-4 rounded-xs border border-gray-100 bg-white p-4 shadow-shopee"
            >
              {/* Product Thumbnail */}
              <div className="relative h-20 w-20 shrink-0 overflow-hidden rounded-xs border bg-gray-50">
                {it.imageUrl ? (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img
                    src={getImageUrl(it.imageUrl)}
                    alt={it.title}
                    className="h-full w-full object-cover"
                  />
                ) : (
                  <div className="grid h-full w-full place-items-center text-xs text-gray-400">
                    🛍️
                  </div>
                )}
              </div>

              {/* Title & Variant Info */}
              <div className="flex-1 min-w-0">
                <Link
                  href={`/listing/${it.listingId}`}
                  className="line-clamp-2 text-xs font-medium text-gray-900 hover:text-brand transition"
                >
                  {it.title}
                </Link>
                {it.variantName && (
                  <span className="mt-1 inline-block rounded-2xs bg-gray-100 px-1.5 py-0.2 text-[10px] text-gray-600">
                    Phân loại: {it.variantName}
                  </span>
                )}
                <div className="mt-2 text-sm font-bold text-brand">
                  {formatPrice(it.unitPrice)}
                </div>
              </div>

              {/* Quantity Controls & Remove */}
              <div className="flex flex-col items-end gap-2">
                <div className="flex items-center rounded-xs border border-gray-200 bg-gray-50">
                  <button
                    type="button"
                    disabled={updatingId === it.id || it.quantity <= 1}
                    onClick={() => handleQuantityChange(it.id, it.quantity - 1)}
                    className="px-2 py-0.5 text-xs font-semibold text-gray-600 hover:bg-gray-200 disabled:opacity-30"
                  >
                    -
                  </button>
                  <span className="w-8 text-center text-xs font-bold text-gray-800">
                    {it.quantity}
                  </span>
                  <button
                    type="button"
                    disabled={updatingId === it.id}
                    onClick={() => handleQuantityChange(it.id, it.quantity + 1)}
                    className="px-2 py-0.5 text-xs font-semibold text-gray-600 hover:bg-gray-200 disabled:opacity-30"
                  >
                    +
                  </button>
                </div>
                <button
                  type="button"
                  disabled={updatingId === it.id}
                  onClick={() => handleRemove(it.id)}
                  className="text-[11px] text-gray-400 hover:text-red-600 transition"
                >
                  Xóa
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Order Summary & Checkout CTA */}
      <div className="h-fit space-y-4 rounded-xs border border-gray-100 bg-white p-5 shadow-shopee">
        <h2 className="text-xs font-bold text-gray-800 uppercase tracking-wider border-b pb-2.5">
          TÓM TẮT ĐƠN HÀNG
        </h2>
        <div className="space-y-2 border-b pb-4 text-xs">
          <div className="flex justify-between text-gray-600">
            <span>Tạm tính:</span>
            <span className="font-semibold text-gray-900">
              {formatPrice(cart.subtotal)}
            </span>
          </div>
          <div className="flex justify-between text-gray-600">
            <span>Phí vận chuyển:</span>
            <span className="text-emerald-600 font-medium">
              Miễn phí (Freeship 0Đ)
            </span>
          </div>
        </div>

        <div className="flex justify-between text-sm font-bold text-gray-900">
          <span>Tổng thanh toán:</span>
          <span className="text-base text-brand">
            {formatPrice(cart.subtotal)}
          </span>
        </div>

        {checkoutEnabled ? (
          <Link
            href="/checkout"
            className="block w-full rounded-xs bg-brand py-3 text-center text-xs font-bold text-white shadow-md transition hover:bg-brand-dark uppercase tracking-wider"
          >
            Mua Hàng ({cart.items.length})
          </Link>
        ) : (
          <div className="space-y-2">
            <button
              type="button"
              disabled
              aria-disabled="true"
              className="block w-full cursor-not-allowed rounded-xs bg-gray-300 py-3 text-center text-xs font-bold text-white uppercase tracking-wider"
            >
              Mua Hàng ({cart.items.length})
            </button>
            <p className="text-center text-[11px] text-gray-500">
              Thanh toán tạm thời không khả dụng. Vui lòng thử lại sau.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
