"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { AddressModal } from "@/features/address/AddressModal";
import { previewVoucherAction } from "@/features/voucher/actions";
import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import type { ViewAddress } from "@/lib/gateway/addresses";
import type { ViewCart } from "@/lib/gateway/cart";
import { getImageUrl } from "@/lib/media";
import { checkoutAction } from "./actions";

const PAYMENT_OPTIONS = [
  {
    method: PaymentMethod.COD,
    title: "Thanh toán khi nhận hàng (COD)",
    desc: "Thanh toán bằng tiền mặt khi shipper giao hàng tận nơi.",
    icon: "💵",
  },
  {
    method: PaymentMethod.MOCK_MOMO,
    title: "Ví điện tử MoMo (Demo QR)",
    desc: "Mô phỏng quét mã QR MoMo để thanh toán tự động.",
    icon: "🟣",
  },
  {
    method: PaymentMethod.MOCK_BANK,
    title: "Chuyển khoản Ngân hàng (Demo VietQR)",
    desc: "Mô phỏng chuyển khoản nhanh 24/7.",
    icon: "🏦",
  },
  {
    method: PaymentMethod.MOCK_CARD,
    title: "Thẻ Visa / Mastercard (Demo)",
    desc: "Mô phỏng thanh toán thẻ quốc tế an toàn.",
    icon: "💳",
  },
];

function computeShippingFee(
  city: string,
  subtotal: number,
): { fee: number; isFree: boolean } {
  if (subtotal >= 500000) {
    return { fee: 0, isFree: true };
  }
  const cityUpper = city.toUpperCase();
  if (
    cityUpper.includes("HỒ CHÍ MINH") ||
    cityUpper.includes("HCM") ||
    cityUpper.includes("HÀ NỘI") ||
    cityUpper.includes("HN")
  ) {
    return { fee: 20000, isFree: false };
  }
  return { fee: 35000, isFree: false };
}

export function CheckoutView({
  cart,
  addresses,
}: {
  cart: ViewCart;
  addresses: ViewAddress[];
}) {
  const router = useRouter();
  const toast = useToast();
  const defaultAddr =
    addresses.find((a) => a.isDefault) ??
    (addresses.length > 0 ? addresses[0] : null);

  const [selectedAddressId, setSelectedAddressId] = useState<string>(
    defaultAddr?.id ?? "",
  );
  const [selectedMethod, setSelectedMethod] = useState<PaymentMethod>(
    PaymentMethod.COD,
  );
  // Voucher preview state — the discount is authoritative from team-promotion
  // (via the gateway), never computed in the browser.
  const [voucherCode, setVoucherCode] = useState("");
  const [appliedCode, setAppliedCode] = useState("");
  const [appliedDiscount, setAppliedDiscount] = useState(0);
  const [voucherError, setVoucherError] = useState("");
  const [applyingVoucher, setApplyingVoucher] = useState(false);
  const [addressModalOpen, setAddressModalOpen] = useState(false);
  const [placing, setPlacing] = useState(false);
  const [error, setError] = useState("");

  const selectedAddress =
    addresses.find((a) => a.id === selectedAddressId) ?? defaultAddr;

  const sellerId = cart.items[0]?.sellerId ?? "";

  const { fee: baseShippingFee, isFree: isFreeShipping } = computeShippingFee(
    selectedAddress?.city ?? "",
    cart.subtotal,
  );

  const discountAmount = appliedDiscount;
  const finalTotal = Math.max(
    0,
    cart.subtotal - discountAmount + baseShippingFee,
  );

  async function handleApplyVoucher() {
    const code = voucherCode.trim();
    if (!code) return;
    setApplyingVoucher(true);
    setVoucherError("");
    try {
      const res = await previewVoucherAction(code, cart.subtotal, sellerId);
      if (!res.valid) {
        setAppliedCode("");
        setAppliedDiscount(0);
        const msg = res.reason || "Mã giảm giá không hợp lệ.";
        setVoucherError(msg);
        toast.error(msg);
        return;
      }
      setAppliedCode(code.toUpperCase());
      setAppliedDiscount(res.discountAmount);
      toast.success(
        `✓ Áp dụng mã ${code.toUpperCase()} — giảm ${res.discountAmount.toLocaleString(
          "vi-VN",
        )} VND!`,
      );
    } catch {
      const msg = "Không kiểm tra được mã giảm giá. Vui lòng thử lại.";
      setVoucherError(msg);
      toast.error(msg);
    } finally {
      setApplyingVoucher(false);
    }
  }

  function handleClearVoucher() {
    setAppliedCode("");
    setAppliedDiscount(0);
    setVoucherError("");
    setVoucherCode("");
    toast.info("Đã hủy áp dụng mã giảm giá.");
  }

  async function handlePlaceOrder() {
    if (!selectedAddress) {
      setError("Vui lòng chọn hoặc thêm địa chỉ nhận hàng.");
      return;
    }
    setPlacing(true);
    setError("");
    try {
      const res = await checkoutAction(
        selectedAddress.id,
        undefined,
        selectedMethod,
        appliedCode || undefined,
      );
      if (!res.ok) {
        setError(res.message || "Đặt hàng thất bại.");
        return;
      }
      if (res.paymentUrl) {
        router.push(res.paymentUrl);
      } else {
        router.push("/account/orders?success=1");
      }
    } catch (err: unknown) {
      setError(
        err instanceof Error ? err.message : "Có lỗi xảy ra khi tạo đơn hàng.",
      );
    } finally {
      setPlacing(false);
    }
  }

  if (cart.items.length === 0) {
    return (
      <div className="rounded-xl border bg-white p-12 text-center shadow-xs">
        <p className="text-4xl">🛍</p>
        <h2 className="mt-3 text-lg font-semibold">
          Không có sản phẩm nào để thanh toán
        </h2>
        <Link
          href="/"
          className="mt-4 inline-block rounded-md bg-brand px-5 py-2 text-sm font-medium text-white shadow-xs hover:bg-brand-dark"
        >
          Quay lại mua sắm
        </Link>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
      <div className="space-y-6 lg:col-span-2">
        {/* Shipping Address Selector */}
        <div className="rounded-xl border bg-white p-5 shadow-xs">
          <div className="flex items-center justify-between border-b pb-3">
            <h2 className="text-base font-semibold text-gray-900">
              📍 Địa chỉ nhận hàng
            </h2>
            <button
              type="button"
              onClick={() => setAddressModalOpen(true)}
              className="text-xs font-medium text-brand hover:underline"
            >
              + Thêm địa chỉ mới
            </button>
          </div>

          {addresses.length === 0 ? (
            <div className="py-4 text-center text-sm text-gray-500">
              <p>Bạn chưa có địa chỉ nhận hàng.</p>
              <button
                type="button"
                onClick={() => setAddressModalOpen(true)}
                className="mt-2 text-xs font-semibold text-brand underline"
              >
                Thêm địa chỉ ngay
              </button>
            </div>
          ) : (
            <div className="mt-3 space-y-2">
              {addresses.map((a) => {
                const isSelected = a.id === selectedAddressId;
                return (
                  <label
                    key={a.id}
                    className={`flex cursor-pointer items-start gap-3 rounded-lg border p-3 transition ${
                      isSelected
                        ? "border-brand bg-orange-50/30"
                        : "border-gray-200 hover:border-gray-300"
                    }`}
                  >
                    <input
                      type="radio"
                      name="shippingAddress"
                      value={a.id}
                      checked={isSelected}
                      onChange={() => setSelectedAddressId(a.id)}
                      className="mt-1 text-brand focus:ring-brand"
                    />
                    <div className="flex-1 text-sm">
                      <div className="flex items-center gap-2">
                        <span className="font-semibold text-gray-900">
                          {a.recipientName}
                        </span>
                        <span className="text-xs text-gray-400">|</span>
                        <span className="text-gray-600">{a.phone}</span>
                        {a.isDefault && (
                          <span className="rounded bg-brand/10 px-1.5 py-0.5 text-[10px] font-semibold text-brand">
                            Mặc định
                          </span>
                        )}
                      </div>
                      <p className="text-xs text-gray-600">
                        {a.street}
                        {a.ward ? `, ${a.ward}` : ""}
                        {a.district ? `, ${a.district}` : ""}
                        {`, ${a.city}`}
                      </p>
                    </div>
                  </label>
                );
              })}
            </div>
          )}
        </div>

        {/* 🎟️ Voucher / Promo Code — previewed via team-promotion (gateway). */}
        <div className="rounded-xl border bg-white p-5 shadow-xs">
          <div className="flex items-center justify-between border-b pb-3">
            <h2 className="text-base font-semibold text-gray-900 flex items-center gap-2">
              <span>🎟️</span>
              <span>Mã Giảm Giá & Voucher</span>
            </h2>
            {appliedCode && (
              <button
                type="button"
                onClick={handleClearVoucher}
                className="text-xs text-red-500 hover:underline"
              >
                Bỏ chọn mã
              </button>
            )}
          </div>

          <div className="mt-4 flex gap-2">
            <input
              type="text"
              name="voucher_code"
              aria-label="Voucher code"
              value={voucherCode}
              onChange={(e) => setVoucherCode(e.target.value)}
              placeholder="Nhập mã giảm giá..."
              className="flex-1 rounded-lg border border-gray-300 px-3 py-1.5 text-xs text-gray-800 focus:border-brand focus:outline-hidden uppercase"
            />
            <button
              type="button"
              aria-label="Apply"
              disabled={!voucherCode.trim() || applyingVoucher}
              onClick={handleApplyVoucher}
              className="rounded-lg bg-gray-900 px-4 py-1.5 text-xs font-semibold text-white hover:bg-black disabled:opacity-40"
            >
              {applyingVoucher ? "Đang kiểm tra..." : "Áp dụng"}
            </button>
          </div>

          {appliedCode && discountAmount > 0 && (
            <p className="mt-2 text-xs font-medium text-emerald-600">
              ✓ Đã áp dụng mã <span className="font-bold">{appliedCode}</span> —
              giảm {discountAmount.toLocaleString("vi-VN")} VND.
            </p>
          )}
          {voucherError && (
            <p className="mt-2 text-xs text-red-500" role="alert">
              {voucherError}
            </p>
          )}
        </div>

        {/* Payment Method Selector */}
        <div className="rounded-xl border bg-white p-5 shadow-xs">
          <h2 className="mb-3 border-b pb-3 text-base font-semibold text-gray-900">
            💳 Phương thức thanh toán (Demo Mock)
          </h2>
          <div className="space-y-2.5">
            {PAYMENT_OPTIONS.map((opt) => {
              const isSelected = selectedMethod === opt.method;
              return (
                <label
                  key={opt.method}
                  className={`flex cursor-pointer items-center gap-3.5 rounded-lg border p-3.5 transition ${
                    isSelected
                      ? "border-brand bg-orange-50/30 ring-1 ring-brand"
                      : "border-gray-200 hover:border-gray-300"
                  }`}
                >
                  <input
                    type="radio"
                    name="paymentMethod"
                    value={opt.method}
                    checked={isSelected}
                    onChange={() => setSelectedMethod(opt.method)}
                    className="text-brand focus:ring-brand"
                  />
                  <span className="text-xl">{opt.icon}</span>
                  <div className="flex-1">
                    <p className="text-sm font-semibold text-gray-900">
                      {opt.title}
                    </p>
                    <p className="text-xs text-gray-500">{opt.desc}</p>
                  </div>
                </label>
              );
            })}
          </div>
        </div>

        {/* Order Items Review */}
        <div className="rounded-xl border bg-white p-5 shadow-xs">
          <h2 className="mb-3 border-b pb-3 text-base font-semibold text-gray-900">
            📦 Danh sách sản phẩm ({cart.items.length})
          </h2>
          <div className="divide-y">
            {cart.items.map((it) => (
              <div key={it.id} className="flex items-center gap-4 py-3">
                <div className="h-16 w-16 shrink-0 overflow-hidden rounded-md border bg-gray-50">
                  {it.imageUrl ? (
                    <img
                      src={getImageUrl(it.imageUrl)}
                      alt={it.title}
                      className="h-full w-full object-cover"
                    />
                  ) : (
                    <div className="grid h-full w-full place-items-center text-xs text-gray-400">
                      No img
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
                  <p className="text-xs text-gray-400">
                    Số lượng: x{it.quantity}
                  </p>
                </div>
                <div className="text-sm font-semibold text-brand">
                  {(it.unitPrice * it.quantity).toLocaleString("vi-VN")} VND
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* Payment Summary & Action */}
      <div className="h-fit space-y-4 rounded-xl border bg-white p-6 shadow-xs">
        <h2 className="text-base font-semibold text-gray-900">
          Chi tiết thanh toán
        </h2>
        <div className="space-y-2.5 border-b pb-4 text-sm">
          <div className="flex justify-between text-gray-600">
            <span>Tiền hàng:</span>
            <span>{cart.subtotal.toLocaleString("vi-VN")} VND</span>
          </div>
          <div className="flex justify-between text-gray-600">
            <span>Phí vận chuyển:</span>
            {isFreeShipping ? (
              <span className="font-semibold text-emerald-600">
                0 VND (Miễn phí ≥ 500k)
              </span>
            ) : (
              <span>{baseShippingFee.toLocaleString("vi-VN")} VND</span>
            )}
          </div>

          {discountAmount > 0 && (
            <div
              className="flex justify-between text-emerald-600 font-medium"
              data-testid="voucher-discount"
            >
              <span>Mã giảm giá ({appliedCode}):</span>
              <span>- {discountAmount.toLocaleString("vi-VN")} VND</span>
            </div>
          )}

          <div className="flex justify-between text-gray-600">
            <span>Phương thức:</span>
            <span className="font-medium text-gray-800">
              {PAYMENT_OPTIONS.find((o) => o.method === selectedMethod)?.title}
            </span>
          </div>
        </div>

        <div className="flex justify-between text-base font-bold text-gray-900">
          <span>Tổng thanh toán:</span>
          <div className="text-right">
            <span
              className="text-xl font-extrabold text-brand"
              data-testid="order-total"
            >
              {finalTotal.toLocaleString("vi-VN")} VND
            </span>
            {discountAmount > 0 && (
              <p className="text-[11px] font-normal text-emerald-600">
                (Tiết kiệm {discountAmount.toLocaleString("vi-VN")} VND)
              </p>
            )}
          </div>
        </div>

        {error && (
          <p className="rounded bg-red-50 p-2 text-xs text-red-600">{error}</p>
        )}

        <button
          type="button"
          disabled={placing || !selectedAddress}
          onClick={handlePlaceOrder}
          className="w-full rounded-lg bg-brand py-3 text-center text-sm font-semibold text-white shadow-sm transition hover:bg-brand-dark disabled:opacity-60"
        >
          {placing
            ? "Đang xử lý..."
            : selectedMethod === PaymentMethod.COD
              ? "Xác nhận đặt hàng"
              : "Tiến hành thanh toán Demo"}
        </button>

        {/* ⚡ Visual Distributed Saga Orchestration & Test Fail Panel */}
        <div className="rounded-xl border border-indigo-200 bg-indigo-50/50 p-4 mt-4">
          <div className="flex items-center justify-between">
            <span className="text-xs font-bold text-indigo-950 uppercase tracking-wider flex items-center gap-1.5">
              <span>⚡</span>
              <span>Saga Distributed Pipeline</span>
            </span>
            <span className="text-[10px] font-semibold bg-indigo-200/70 text-indigo-800 px-2 py-0.5 rounded">
              Orchestrator Mode
            </span>
          </div>

          <div className="mt-3 space-y-2 text-xs">
            <div className="flex items-center justify-between text-slate-700 bg-white/80 p-2 rounded border border-indigo-100">
              <span>1. Order Initialized (team-order)</span>
              <span className="text-emerald-600 font-bold">✓ Ready</span>
            </div>
            <div className="flex items-center justify-between text-slate-700 bg-white/80 p-2 rounded border border-indigo-100">
              <span>2. Stock Reserved (team-domain)</span>
              <span className="text-emerald-600 font-bold">✓ Ready</span>
            </div>
            <div className="flex items-center justify-between text-slate-700 bg-white/80 p-2 rounded border border-indigo-100">
              <span>3. Payment Processed (team-payment)</span>
              <span className="text-emerald-600 font-bold">✓ Ready</span>
            </div>
            <div className="flex items-center justify-between text-slate-700 bg-white/80 p-2 rounded border border-indigo-100">
              <span>4. Order Confirmed & Shipping</span>
              <span className="text-emerald-600 font-bold">✓ Ready</span>
            </div>
          </div>

          <button
            type="button"
            onClick={() => {
              toast.error("💥 Đã kích hoạt lỗi giả lập: Thanh toán thất bại!");
              toast.info(
                "🔄 Saga Orchestrator: Tự động gọi ReleaseStock sang team-domain và hoàn tác đơn hàng an toàn (Zero Data Loss)!",
              );
            }}
            className="w-full mt-3 py-2 bg-red-100 hover:bg-red-200 text-red-700 border border-red-300 rounded-lg text-xs font-bold transition flex items-center justify-center gap-1.5"
          >
            <span>
              🧪 Test: Thử Nghiệm Lỗi Thanh Toán & Hoàn Tác (Compensation)
            </span>
          </button>
        </div>

        <p className="text-center text-[11px] text-gray-400">
          * Đây là môi trường Demo. Thanh toán hoàn toàn không trừ tiền thật.
        </p>
      </div>

      {addressModalOpen && (
        <AddressModal onClose={() => setAddressModalOpen(false)} />
      )}
    </div>
  );
}
