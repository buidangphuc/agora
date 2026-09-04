"use client";

import Link from "next/link";
import { useState } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";
import { getImageUrl } from "@/lib/media";
import { updateOrderStatusAction } from "./actions";

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

export function SellerOrdersList({
  initialOrders,
}: { initialOrders: ViewOrder[] }) {
  const [orders, setOrders] = useState<ViewOrder[]>(initialOrders);
  const [statusTab, setStatusTab] = useState<OrderStatus | 0>(0);
  const [updatingId, setUpdatingId] = useState<string | null>(null);
  const toast = useToast();

  async function handleAdvance(
    orderId: string,
    nextStatus: OrderStatus,
    trackingNumber?: string,
  ) {
    setUpdatingId(orderId);
    try {
      const res = await updateOrderStatusAction(
        orderId,
        nextStatus,
        trackingNumber,
      );
      if (res.ok) {
        let statusText = "Đang giao hàng";
        if (nextStatus === OrderStatus.COMPLETED) {
          statusText = "Đã hoàn thành";
          toast.success("✓ Đã hoàn tất đơn hàng!");
        } else if (nextStatus === OrderStatus.SHIPPED) {
          toast.success("✓ Đã xác nhận gửi hàng thành công!");
        }
        setOrders((prev) =>
          prev.map((o) =>
            o.id === orderId
              ? {
                  ...o,
                  status: nextStatus,
                  statusText,
                  trackingNumber: trackingNumber || o.trackingNumber,
                }
              : o,
          ),
        );
      } else {
        toast.error("Cập nhật trạng thái đơn hàng thất bại.");
      }
    } catch {
      toast.error("Có lỗi xảy ra khi cập nhật đơn hàng.");
    } finally {
      setUpdatingId(null);
    }
  }

  const filteredOrders =
    statusTab === 0 ? orders : orders.filter((o) => o.status === statusTab);

  const pendingCount = orders.filter(
    (o) => o.status === OrderStatus.PENDING,
  ).length;
  const shippedCount = orders.filter(
    (o) => o.status === OrderStatus.SHIPPED,
  ).length;
  const completedCount = orders.filter(
    (o) => o.status === OrderStatus.COMPLETED,
  ).length;

  if (orders.length === 0) {
    return (
      <div className="rounded-xl border border-gray-200 bg-white p-12 text-center shadow-2xs">
        <span className="text-4xl">📋</span>
        <h2 className="mt-3 text-base font-bold text-gray-800">
          Chưa có đơn hàng nào từ người mua
        </h2>
        <p className="mt-1 text-xs text-gray-500">
          Khi có khách đặt mua sản phẩm của shop, đơn hàng sẽ hiển thị tại đây.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-gray-200 bg-white p-5 rounded-xl shadow-2xs">
        <div>
          <h1 className="text-lg font-bold text-gray-900">Quản lý đơn hàng</h1>
          <p className="text-xs text-gray-500 mt-0.5">
            Theo dõi, đóng gói và xác nhận gửi hàng tới người mua
          </p>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex border-b border-gray-200 bg-white px-3 rounded-xl shadow-2xs text-xs font-semibold text-gray-600 gap-2">
        <button
          type="button"
          onClick={() => setStatusTab(0)}
          className={`py-3 px-3 border-b-2 transition ${
            statusTab === 0
              ? "border-brand text-brand font-bold"
              : "border-transparent hover:text-gray-900"
          }`}
        >
          Tất cả ({orders.length})
        </button>
        <button
          type="button"
          onClick={() => setStatusTab(OrderStatus.PENDING)}
          className={`py-3 px-3 border-b-2 transition ${
            statusTab === OrderStatus.PENDING
              ? "border-brand text-brand font-bold"
              : "border-transparent hover:text-gray-900"
          }`}
        >
          Chờ xử lý ({pendingCount})
        </button>
        <button
          type="button"
          onClick={() => setStatusTab(OrderStatus.SHIPPED)}
          className={`py-3 px-3 border-b-2 transition ${
            statusTab === OrderStatus.SHIPPED
              ? "border-brand text-brand font-bold"
              : "border-transparent hover:text-gray-900"
          }`}
        >
          Đang giao ({shippedCount})
        </button>
        <button
          type="button"
          onClick={() => setStatusTab(OrderStatus.COMPLETED)}
          className={`py-3 px-3 border-b-2 transition ${
            statusTab === OrderStatus.COMPLETED
              ? "border-brand text-brand font-bold"
              : "border-transparent hover:text-gray-900"
          }`}
        >
          Đã hoàn thành ({completedCount})
        </button>
      </div>

      {/* Orders List */}
      <div className="space-y-4">
        {filteredOrders.length === 0 ? (
          <div className="rounded-xl border border-gray-200 bg-white p-10 text-center text-xs text-gray-400">
            Không có đơn hàng nào trong mục này.
          </div>
        ) : (
          filteredOrders.map((o) => (
            <div
              key={o.id}
              className="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-2xs"
            >
              {/* Header */}
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

              {/* Items */}
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
                          🛍️
                        </div>
                      )}
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="line-clamp-1 text-sm font-medium text-gray-900">
                        {it.title}
                      </p>
                      {it.variantName && (
                        <p className="text-xs text-gray-500">
                          Phân loại: {it.variantName}
                        </p>
                      )}
                      <p className="text-xs text-gray-400">x{it.quantity}</p>
                    </div>
                    <div className="text-sm font-semibold text-gray-800">
                      {(it.unitPrice * it.quantity).toLocaleString("vi-VN")} VND
                    </div>
                  </div>
                ))}
              </div>

              {/* Shipping Info & Actions */}
              <div className="border-t bg-gray-50/30 px-4 py-3 text-sm">
                <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
                  <div className="text-xs text-gray-600">
                    <p>
                      <span className="font-semibold">Người nhận: </span>
                      {o.recipientName} ({o.phone})
                    </p>
                    <p className="text-gray-500">Địa chỉ: {o.addressFull}</p>
                    <p className="mt-1 text-gray-400">
                      Tiền hàng: {o.itemsSubtotal.toLocaleString("vi-VN")} VND ·
                      Phí ship: {o.shippingFee.toLocaleString("vi-VN")} VND
                    </p>
                    {o.trackingNumber && (
                      <p className="mt-0.5 font-medium text-purple-700">
                        Mã vận đơn: {o.trackingNumber}
                      </p>
                    )}
                  </div>

                  <div className="flex items-center gap-3">
                    <div>
                      <span className="text-xs text-gray-500">Doanh thu: </span>
                      <span className="text-base font-bold text-brand">
                        {o.totalAmount.toLocaleString("vi-VN")} VND
                      </span>
                    </div>

                    {o.status === OrderStatus.PENDING && (
                      <button
                        type="button"
                        disabled={updatingId === o.id}
                        onClick={() =>
                          handleAdvance(
                            o.id,
                            OrderStatus.SHIPPED,
                            `VNPOST-${o.id.slice(0, 6).toUpperCase()}`,
                          )
                        }
                        className="rounded-lg bg-brand px-4 py-2 text-xs font-semibold text-white shadow-xs hover:bg-brand-dark transition disabled:opacity-40"
                      >
                        {updatingId === o.id
                          ? "Đang cập nhật..."
                          : "🚚 Xác nhận gửi hàng"}
                      </button>
                    )}

                    {o.status === OrderStatus.SHIPPED && (
                      <button
                        type="button"
                        disabled={updatingId === o.id}
                        onClick={() =>
                          handleAdvance(o.id, OrderStatus.COMPLETED)
                        }
                        className="rounded-lg bg-emerald-600 px-4 py-2 text-xs font-semibold text-white shadow-xs hover:bg-emerald-700 transition disabled:opacity-40"
                      >
                        {updatingId === o.id
                          ? "Đang cập nhật..."
                          : "✅ Hoàn tất đơn"}
                      </button>
                    )}
                  </div>
                </div>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
