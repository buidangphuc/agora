"use client";

import Link from "next/link";
import { useState } from "react";

import { ReviewModal } from "@/features/review/ReviewModal";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";
import { getImageUrl } from "@/lib/media";

export function OrderDetailView({ order }: { order: ViewOrder }) {
  const [reviewingItem, setReviewingItem] = useState<{
    listingId: string;
    orderId: string;
    productTitle: string;
  } | null>(null);

  const [showCancelModal, setShowCancelModal] = useState(false);
  const [cancelReason, setCancelReason] = useState("changed_mind");
  const [cancelDone, setCancelDone] = useState(false);

  // Stepper logic
  const isPending = order.status === OrderStatus.PENDING;
  const isPaid = order.status === OrderStatus.PAID;
  const isShipped = order.status === OrderStatus.SHIPPED;
  const isCompleted = order.status === OrderStatus.COMPLETED;
  const isCancelled = order.status === OrderStatus.CANCELLED;

  const currentStepIndex = isCancelled
    ? -1
    : isCompleted
      ? 5
      : isShipped
        ? 4
        : isPaid
          ? 3
          : isPending
            ? 2
            : 1;

  const steps = [
    { title: "Đã Đặt Hàng", desc: "Đơn hàng đã được tạo", icon: "📝" },
    { title: "Đã Thanh Toán", desc: "Xác nhận thanh toán", icon: "💳" },
    { title: "Shop Đóng Gói", desc: "Chuẩn bị đơn hàng", icon: "📦" },
    { title: "Đang Vận Chuyển", desc: "SPX Express đang giao", icon: "🚚" },
    { title: "Đã Nhận Hàng", desc: "Giao hàng thành công", icon: "✅" },
  ];

  return (
    <div className="space-y-6">
      {/* ── Breadcrumb & Header ── */}
      <div className="flex items-center justify-between border-b bg-white p-5 rounded-xl shadow-2xs">
        <div>
          <nav className="flex items-center gap-2 text-xs text-gray-500 mb-1">
            <Link href="/account/orders" className="hover:text-brand">
              Đơn hàng của tôi
            </Link>
            <span>&gt;</span>
            <span className="font-semibold text-gray-800">
              Chi tiết #{order.id.slice(0, 8)}
            </span>
          </nav>
          <h1 className="text-lg font-bold text-gray-900 flex items-center gap-2">
            <span>Chi tiết đơn hàng: #{order.id}</span>
          </h1>
          <p className="text-xs text-gray-500 mt-0.5">
            Ngày đặt: {order.createdAt}
          </p>
        </div>

        <div className="text-right">
          <span className="rounded bg-brand/10 px-3 py-1 text-xs font-bold text-brand">
            {order.statusText}
          </span>
        </div>
      </div>

      {/* ── Visual 5-Step Timeline Stepper ── */}
      <div className="rounded-xl border border-gray-100 bg-white p-6 shadow-2xs">
        <h2 className="text-xs font-bold uppercase tracking-wider text-gray-700 mb-6">
          HÀNH TRÌNH ĐƠN HÀNG
        </h2>

        {isCancelled ? (
          <div className="rounded-lg bg-red-50 p-4 text-center text-xs text-red-600 font-semibold">
            ❌ Đơn hàng này đã bị hủy. Tồn kho sản phẩm đã được tự động hoàn
            lại.
          </div>
        ) : (
          <div className="relative flex items-center justify-between">
            {/* Background Line */}
            <div className="absolute left-0 top-1/2 -translate-y-1/2 h-1 w-full bg-gray-200 z-0" />
            <div
              className="absolute left-0 top-1/2 -translate-y-1/2 h-1 bg-brand transition-all duration-500 z-0"
              style={{
                width: `${((currentStepIndex - 1) / (steps.length - 1)) * 100}%`,
              }}
            />

            {/* Stepper Nodes */}
            {steps.map((step, idx) => {
              const stepNum = idx + 1;
              const isPastOrCurrent = stepNum <= currentStepIndex;
              const isCurrent = stepNum === currentStepIndex;

              return (
                <div
                  key={step.title}
                  className="relative z-10 flex flex-col items-center text-center max-w-[100px]"
                >
                  <div
                    className={`grid h-10 w-10 place-items-center rounded-full text-base transition-all duration-300 ${
                      isCurrent
                        ? "bg-brand text-white ring-4 ring-orange-100 shadow-md scale-110"
                        : isPastOrCurrent
                          ? "bg-brand text-white shadow-xs"
                          : "bg-gray-100 text-gray-400 border border-gray-300"
                    }`}
                  >
                    {step.icon}
                  </div>
                  <span
                    className={`mt-2 text-xs font-bold ${
                      isPastOrCurrent ? "text-gray-900" : "text-gray-400"
                    }`}
                  >
                    {step.title}
                  </span>
                  <span className="hidden sm:inline text-[10px] text-gray-400">
                    {step.desc}
                  </span>
                </div>
              );
            })}
          </div>
        )}

        {order.trackingNumber && (
          <div className="mt-6 rounded-lg bg-orange-50/50 p-3.5 border border-brand/20 flex flex-col sm:flex-row sm:items-center justify-between text-xs gap-2">
            <div>
              <span className="font-semibold text-gray-800">
                Đơn vị vận chuyển:{" "}
              </span>
              <span className="text-brand font-bold">SPX Express Hỏa Tốc</span>
              <p className="text-[11px] text-gray-500 mt-0.5">
                Mã vận đơn: <strong>{order.trackingNumber}</strong>
              </p>
            </div>
            <span className="rounded bg-emerald-100 text-emerald-800 px-2.5 py-1 text-[11px] font-semibold">
              🚚 Đang trên đường giao đến bạn
            </span>
          </div>
        )}
      </div>

      {/* ── Shipping & Payment Info ── */}
      <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
        {/* Recipient Address */}
        <div className="rounded-xl border border-gray-100 bg-white p-5 shadow-2xs space-y-2 text-xs">
          <h3 className="font-bold uppercase tracking-wider text-gray-900 border-b pb-2">
            📍 ĐỊA CHỈ NHẬN HÀNG
          </h3>
          <p className="font-bold text-sm text-gray-900">
            {order.recipientName} ({order.phone})
          </p>
          <p className="text-gray-600 leading-relaxed">{order.addressFull}</p>
        </div>

        {/* Payment Info */}
        <div className="rounded-xl border border-gray-100 bg-white p-5 shadow-2xs space-y-2 text-xs">
          <h3 className="font-bold uppercase tracking-wider text-gray-900 border-b pb-2">
            💳 HÌNH THỨC THANH TOÁN
          </h3>
          <p className="font-bold text-sm text-gray-900">
            {order.paymentMethodText}
          </p>
          <p className="text-gray-500">
            Thanh toán an toàn, bảo vệ người mua 100% với chính sách đổi trả
            miễn phí.
          </p>
        </div>
      </div>

      {/* ── Order Items ── */}
      <div className="rounded-xl border border-gray-100 bg-white p-5 shadow-2xs">
        <h3 className="font-bold uppercase tracking-wider text-gray-900 border-b pb-3 text-xs">
          📦 DANH SÁCH SẢN PHẨM ({order.items.length})
        </h3>
        <div className="divide-y">
          {order.items.map((it) => (
            <div key={it.id} className="flex items-center gap-4 py-3.5">
              <div className="h-16 w-16 shrink-0 overflow-hidden rounded-md border border-gray-100 bg-gray-50">
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
              <div className="flex-1 min-w-0">
                <Link
                  href={`/listing/${it.listingId}`}
                  className="line-clamp-1 text-sm font-semibold text-gray-900 hover:text-brand"
                >
                  {it.title}
                </Link>
                {it.variantName && (
                  <p className="text-xs text-gray-500 mt-0.5">
                    Phân loại: {it.variantName}
                  </p>
                )}
                <p className="text-xs text-gray-400">
                  Số lượng: x{it.quantity}
                </p>
              </div>
              <div className="text-right">
                <div className="text-sm font-bold text-brand">
                  {(it.unitPrice * it.quantity).toLocaleString("vi-VN")} VND
                </div>
                {order.status === OrderStatus.COMPLETED && (
                  <button
                    type="button"
                    onClick={() =>
                      setReviewingItem({
                        listingId: it.listingId,
                        orderId: order.id,
                        productTitle: it.title,
                      })
                    }
                    className="mt-1.5 inline-block rounded bg-orange-50 px-2.5 py-1 text-xs font-semibold text-brand hover:bg-orange-100 transition"
                  >
                    ⭐ Đánh giá sản phẩm
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>

        {/* Price Breakdown */}
        <div className="border-t pt-4 space-y-2 text-xs text-gray-600">
          <div className="flex justify-between">
            <span>Tổng tiền hàng:</span>
            <span>{order.itemsSubtotal.toLocaleString("vi-VN")} VND</span>
          </div>
          <div className="flex justify-between">
            <span>Phí vận chuyển:</span>
            <span>{order.shippingFee.toLocaleString("vi-VN")} VND</span>
          </div>
          <div className="flex justify-between font-bold text-sm text-gray-900 pt-2 border-t">
            <span>Tổng thanh toán:</span>
            <span className="text-base text-brand">
              {order.totalAmount.toLocaleString("vi-VN")} VND
            </span>
          </div>
        </div>

        {/* Bottom Actions */}
        <div className="mt-6 flex flex-wrap items-center justify-end gap-3 pt-4 border-t">
          {!isCancelled && !isCompleted && (
            <button
              type="button"
              onClick={() => setShowCancelModal(true)}
              className="rounded-lg border border-red-200 bg-red-50/50 px-4 py-2 text-xs font-semibold text-red-600 hover:bg-red-100 transition"
            >
              ❌ Hủy đơn / Yêu cầu Trả hàng (RMA)
            </button>
          )}

          <Link
            href="/chat"
            className="rounded-lg border border-gray-300 px-4 py-2 text-xs font-semibold text-gray-700 hover:border-brand hover:text-brand transition"
          >
            💬 Liên hệ Shop
          </Link>
          <Link
            href="/"
            className="rounded-lg bg-brand px-5 py-2 text-xs font-semibold text-white hover:bg-brand-dark shadow-xs transition"
          >
            Mua lại sản phẩm
          </Link>
        </div>
      </div>

      {/* Cancellation / RMA Modal */}
      {showCancelModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-2xl space-y-4">
            <h3 className="text-base font-bold text-gray-900">
              Yêu Cầu Hủy Đơn / Trả Hàng Hoàn Tiền (RMA)
            </h3>
            <p className="text-xs text-gray-500">
              Vui lòng chọn lý do để hệ thống xử lý hoàn tiền tự động và trả lại
              tồn kho cho người bán:
            </p>

            <select
              value={cancelReason}
              onChange={(e) => setCancelReason(e.target.value)}
              className="w-full rounded-lg border border-gray-200 p-2.5 text-xs text-gray-800 outline-none focus:border-brand"
            >
              <option value="changed_mind">
                Tôi muốn thay đổi địa chỉ hoặc đổi ý
              </option>
              <option value="wrong_item">
                Shop tư vấn hoặc giao sai phân loại
              </option>
              <option value="damaged">Sản phẩm bị lỗi / hư hỏng</option>
              <option value="delivery_delay">
                Thời gian giao hàng quá lâu
              </option>
            </select>

            {cancelDone ? (
              <div className="rounded-lg bg-emerald-50 p-3 text-xs font-bold text-emerald-800">
                ✓ Đã hủy đơn và kích hoạt hoàn tiền tự động thành công!
              </div>
            ) : (
              <div className="flex justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCancelModal(false)}
                  className="px-4 py-2 rounded-lg border border-gray-200 text-xs font-semibold text-gray-600 hover:bg-gray-50"
                >
                  Đóng
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setCancelDone(true);
                    setTimeout(() => {
                      setShowCancelModal(false);
                      window.location.reload();
                    }, 1500);
                  }}
                  className="px-4 py-2 rounded-lg bg-red-600 hover:bg-red-700 text-white text-xs font-bold shadow-sm"
                >
                  Xác Nhận Hủy Đơn
                </button>
              </div>
            )}
          </div>
        </div>
      )}

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
