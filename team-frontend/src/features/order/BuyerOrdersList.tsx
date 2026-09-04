"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { ReviewModal } from "@/features/review/ReviewModal";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";
import { getImageUrl } from "@/lib/media";
import { cancelOrderAction, reorderAction } from "./actions";

function getStatusBadge(status: OrderStatus, text: string) {
  switch (status) {
    case OrderStatus.PENDING:
      return (
        <span className="rounded bg-yellow-100 px-2 py-0.5 text-xs font-semibold text-yellow-800">
          ⏳ {text}
        </span>
      );
    case OrderStatus.PAID:
      return (
        <span className="rounded bg-blue-100 px-2 py-0.5 text-xs font-semibold text-blue-800">
          💳 {text}
        </span>
      );
    case OrderStatus.SHIPPED:
      return (
        <span className="rounded bg-purple-100 px-2 py-0.5 text-xs font-semibold text-purple-800">
          🚚 {text}
        </span>
      );
    case OrderStatus.COMPLETED:
      return (
        <span className="rounded bg-green-100 px-2 py-0.5 text-xs font-semibold text-green-800">
          ✅ {text}
        </span>
      );
    case OrderStatus.CANCELLED:
      return (
        <span className="rounded bg-gray-100 px-2 py-0.5 text-xs font-semibold text-gray-600">
          ❌ {text}
        </span>
      );
    default:
      return (
        <span className="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600">
          {text}
        </span>
      );
  }
}

export function BuyerOrdersList({
  initialOrders,
}: { initialOrders: ViewOrder[] }) {
  const [orders, setOrders] = useState<ViewOrder[]>(initialOrders);
  const [cancellingId, setCancellingId] = useState<string | null>(null);
  const [reorderingId, setReorderingId] = useState<string | null>(null);
  const [, startReorder] = useTransition();
  const toast = useToast();
  const router = useRouter();
  const [reviewingItem, setReviewingItem] = useState<{
    listingId: string;
    orderId: string;
    productTitle: string;
  } | null>(null);

  async function handleCancel(orderId: string) {
    if (!confirm("Bạn có chắc chắn muốn hủy đơn hàng này?")) return;
    setCancellingId(orderId);
    try {
      const res = await cancelOrderAction(orderId);
      if (res.ok) {
        setOrders((prev) =>
          prev.map((o) =>
            o.id === orderId
              ? { ...o, status: OrderStatus.CANCELLED, statusText: "Đã hủy" }
              : o,
          ),
        );
      }
    } finally {
      setCancellingId(null);
    }
  }

  function handleReorder(orderId: string) {
    setReorderingId(orderId);
    startReorder(async () => {
      const res = await reorderAction(orderId);
      setReorderingId(null);
      if (res.ok) {
        toast.success("✓ Đã thêm lại sản phẩm vào giỏ hàng.");
        router.push("/cart");
      } else {
        toast.error(res.message || "Có lỗi xảy ra.");
      }
    });
  }

  if (orders.length === 0) {
    return (
      <div className="rounded-xl border bg-white p-12 text-center shadow-xs">
        <p className="text-4xl">📦</p>
        <h2 className="mt-3 text-lg font-semibold text-gray-800">
          Bạn chưa có đơn hàng nào
        </h2>
        <p className="mt-1 text-sm text-gray-500">
          Hãy khám phá các sản phẩm và đặt đơn đầu tiên nhé.
        </p>
        <Link
          href="/"
          className="mt-5 inline-block rounded-md bg-brand px-5 py-2 text-sm font-medium text-white shadow-sm hover:bg-brand-dark"
        >
          Mua sắm ngay
        </Link>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <h1 className="text-xl font-bold text-gray-900">Đơn hàng của tôi</h1>
      <div className="space-y-4">
        {orders.map((o) => {
          const isOnlinePending =
            o.status === OrderStatus.PENDING &&
            o.paymentMethod !== PaymentMethod.COD &&
            o.paymentMethod !== PaymentMethod.UNSPECIFIED;

          return (
            <div
              key={o.id}
              className="overflow-hidden rounded-xl border bg-white shadow-xs"
            >
              {/* Order Header */}
              <div className="flex items-center justify-between border-b bg-gray-50/50 px-4 py-3 text-xs text-gray-500">
                <div className="flex items-center gap-2">
                  <span className="font-semibold text-gray-700">
                    Mã đơn: #{o.id.slice(0, 8)}
                  </span>
                  <span>·</span>
                  <span>{o.createdAt}</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="rounded bg-gray-100 px-2 py-0.5 text-[11px] font-medium text-gray-600">
                    {o.paymentMethodText}
                  </span>
                  <div>{getStatusBadge(o.status, o.statusText)}</div>
                </div>
              </div>

              {/* Order Items */}
              <div className="divide-y px-4">
                {o.items.map((it) => (
                  <div key={it.id} className="flex items-center gap-3 py-3">
                    <div className="h-14 w-14 shrink-0 overflow-hidden rounded-md border bg-gray-50">
                      {it.imageUrl ? (
                        <img
                          src={getImageUrl(it.imageUrl)}
                          alt={it.title}
                          className="h-full w-full object-cover"
                        />
                      ) : (
                        <div className="grid h-full w-full place-items-center text-[10px] text-gray-400">
                          No img
                        </div>
                      )}
                    </div>
                    <div className="flex-1 min-w-0">
                      <Link
                        href={`/listing/${it.listingId}`}
                        className="line-clamp-1 text-sm font-medium text-gray-900 hover:text-brand"
                      >
                        {it.title}
                      </Link>
                      {it.variantName && (
                        <p className="text-xs text-gray-500">
                          Phân loại: {it.variantName}
                        </p>
                      )}
                      <p className="text-xs text-gray-400">x{it.quantity}</p>
                    </div>
                    <div className="text-right">
                      <div className="text-sm font-semibold text-gray-800">
                        {(it.unitPrice * it.quantity).toLocaleString("vi-VN")}{" "}
                        VND
                      </div>
                      {o.status === OrderStatus.COMPLETED && (
                        <button
                          type="button"
                          onClick={() =>
                            setReviewingItem({
                              listingId: it.listingId,
                              orderId: o.id,
                              productTitle: it.title,
                            })
                          }
                          className="mt-1 inline-block rounded bg-orange-50 px-2 py-0.5 text-[11px] font-semibold text-brand hover:bg-orange-100"
                        >
                          ⭐ Đánh giá
                        </button>
                      )}
                    </div>
                  </div>
                ))}
              </div>

              {/* Order Footer & Actions */}
              <div className="border-t bg-gray-50/30 px-4 py-3 text-sm">
                <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
                  <div className="text-xs text-gray-500">
                    <p>
                      <span className="font-medium text-gray-700">
                        Giao đến:{" "}
                      </span>
                      {o.recipientName} ({o.phone}) - {o.addressFull}
                    </p>
                    <p className="mt-1 text-gray-400">
                      Tiền hàng: {o.itemsSubtotal.toLocaleString("vi-VN")} VND ·
                      Phí ship: {o.shippingFee.toLocaleString("vi-VN")} VND
                    </p>
                    {o.trackingNumber && (
                      <p className="mt-0.5 text-purple-700">
                        Mã vận đơn: {o.trackingNumber}
                      </p>
                    )}
                  </div>

                  <div className="flex items-center gap-3">
                    <div>
                      <span className="text-xs text-gray-500">
                        Tổng thanh toán:{" "}
                      </span>
                      <span className="text-base font-bold text-brand">
                        {o.totalAmount.toLocaleString("vi-VN")} VND
                      </span>
                    </div>

                    <Link
                      href={`/account/orders/${o.id}`}
                      className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-xs font-semibold text-gray-700 shadow-xs hover:border-brand hover:text-brand"
                    >
                      Xem chi tiết
                    </Link>

                    <button
                      type="button"
                      disabled={reorderingId === o.id}
                      onClick={() => handleReorder(o.id)}
                      className="rounded-md border border-brand bg-white px-3 py-1.5 text-xs font-semibold text-brand shadow-xs hover:bg-orange-50 disabled:opacity-40"
                    >
                      {reorderingId === o.id ? "Đang thêm..." : "🛒 Mua lại"}
                    </button>

                    {isOnlinePending && (
                      <Link
                        href={`/checkout/pay/${o.id}`}
                        className="rounded-md bg-brand px-3 py-1.5 text-xs font-semibold text-white shadow-xs hover:bg-brand-dark"
                      >
                        Thanh toán ngay
                      </Link>
                    )}

                    {o.status === OrderStatus.PENDING && (
                      <button
                        type="button"
                        disabled={cancellingId === o.id}
                        onClick={() => handleCancel(o.id)}
                        className="rounded-md border border-red-200 bg-white px-3 py-1.5 text-xs font-medium text-red-600 shadow-xs hover:bg-red-50 disabled:opacity-40"
                      >
                        {cancellingId === o.id ? "Đang hủy..." : "Hủy đơn"}
                      </button>
                    )}
                  </div>
                </div>
              </div>
            </div>
          );
        })}
      </div>

      {reviewingItem && (
        <ReviewModal
          listingId={reviewingItem.listingId}
          orderId={reviewingItem.orderId}
          productTitle={reviewingItem.productTitle}
          onClose={() => setReviewingItem(null)}
        />
      )}
    </div>
  );
}
