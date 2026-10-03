"use client";

import Link from "next/link";
import { useState } from "react";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { PriceTag } from "@/components/ui/PriceTag";
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
      <Card className="rounded-2xl p-16 text-center border-gray-200/80">
        <div className="mx-auto flex h-20 w-20 items-center justify-center rounded-full bg-primary-50 text-4xl">
          🛒
        </div>
        <h2 className="mt-5 text-lg font-bold text-gray-900">
          Giỏ hàng của bạn đang trống
        </h2>
        <p className="mt-1.5 text-xs text-gray-500 max-w-sm mx-auto">
          Khám phá hàng triệu ưu đãi công nghệ, thời trang và săn deal giá hời
          ngay hôm nay!
        </p>
        <div className="mt-6">
          <Link href="/">
            <Button
              variant="primary"
              size="lg"
              className="font-semibold uppercase tracking-wider"
            >
              Khám Phá Mua Sắm
            </Button>
          </Link>
        </div>
      </Card>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
      {/* Items list */}
      <div className="space-y-3 lg:col-span-2">
        <div className="flex items-center justify-between border-b border-gray-100 bg-white px-5 py-4 rounded-xl shadow-preline-card border border-gray-200/80">
          <h1 className="text-sm font-bold text-gray-900 uppercase tracking-wide">
            Giỏ Hàng ({cart.items.length} sản phẩm)
          </h1>
          <button
            type="button"
            onClick={handleClear}
            className="text-xs text-gray-400 hover:text-red-600 transition cursor-pointer"
          >
            Xóa tất cả
          </button>
        </div>

        <div className="space-y-3">
          {cart.items.map((it) => (
            <div
              key={it.id}
              className="flex items-center gap-4 rounded-xl border border-gray-200/80 bg-white p-4 shadow-preline-card transition hover:border-gray-300"
            >
              {/* Product Thumbnail */}
              <div className="relative h-20 w-20 shrink-0 overflow-hidden rounded-lg border border-gray-100 bg-gray-50">
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
                  className="line-clamp-2 text-xs font-medium text-gray-900 hover:text-primary-600 transition"
                >
                  {it.title}
                </Link>
                {it.variantName && (
                  <div className="mt-1">
                    <Badge variant="neutral" size="xs">
                      Phân loại: {it.variantName}
                    </Badge>
                  </div>
                )}
                <div className="mt-2">
                  <PriceTag price={it.unitPrice} size="sm" />
                </div>
              </div>

              {/* Quantity Controls & Remove */}
              <div className="flex flex-col items-end gap-2.5">
                <div className="flex items-center rounded-lg border border-gray-200 bg-gray-50/80 overflow-hidden shadow-2xs">
                  <button
                    type="button"
                    disabled={updatingId === it.id || it.quantity <= 1}
                    onClick={() => handleQuantityChange(it.id, it.quantity - 1)}
                    className="px-2.5 py-1 text-xs font-semibold text-gray-600 hover:bg-gray-200 disabled:opacity-30 transition cursor-pointer"
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
                    className="px-2.5 py-1 text-xs font-semibold text-gray-600 hover:bg-gray-200 disabled:opacity-30 transition cursor-pointer"
                  >
                    +
                  </button>
                </div>
                <button
                  type="button"
                  disabled={updatingId === it.id}
                  onClick={() => handleRemove(it.id)}
                  className="text-xs text-gray-400 hover:text-red-600 transition cursor-pointer font-medium"
                >
                  Xóa
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Order Summary & Checkout CTA */}
      <Card className="h-fit rounded-2xl border-gray-200/80 p-6 space-y-4">
        <h2 className="text-xs font-bold text-gray-900 uppercase tracking-wider border-b border-gray-100 pb-3">
          TÓM TẮT ĐƠN HÀNG
        </h2>
        <div className="space-y-2.5 border-b border-gray-100 pb-4 text-xs">
          <div className="flex justify-between text-gray-600">
            <span>Tạm tính ({cart.items.length} món):</span>
            <span className="font-semibold text-gray-900">
              {formatPrice(cart.subtotal)}
            </span>
          </div>
          <div className="flex justify-between text-gray-600">
            <span>Phí vận chuyển:</span>
            <Badge variant="success" size="xs">
              Freeship 0Đ
            </Badge>
          </div>
        </div>

        <div className="flex justify-between items-baseline text-sm font-bold text-gray-900 pt-1">
          <span>Tổng thanh toán:</span>
          <PriceTag price={cart.subtotal} size="lg" />
        </div>

        {checkoutEnabled ? (
          <Link href="/checkout" className="block w-full pt-2">
            <Button
              variant="primary"
              size="lg"
              className="w-full font-bold uppercase tracking-wider shadow-md"
            >
              Tiến Hành Mua Hàng ({cart.items.length})
            </Button>
          </Link>
        ) : (
          <div className="space-y-2 pt-2">
            <Button
              variant="secondary"
              size="lg"
              disabled
              className="w-full font-bold uppercase tracking-wider opacity-60 cursor-not-allowed"
            >
              Mua Hàng ({cart.items.length})
            </Button>
            <p className="text-center text-[11px] text-gray-500">
              Thanh toán tạm thời không khả dụng. Vui lòng thử lại sau.
            </p>
          </div>
        )}
      </Card>
    </div>
  );
}
