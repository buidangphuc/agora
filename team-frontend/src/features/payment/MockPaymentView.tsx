"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { processMockPaymentAction } from "@/features/order/actions";
import {
  PaymentMethod,
  PaymentStatus,
} from "@/generated/platform/payment/v1/payment_pb.js";
import type { ViewOrder } from "@/lib/gateway/orders";
import type { ViewPaymentTransaction } from "@/lib/gateway/payment";

export function MockPaymentView({
  order,
  transaction,
}: {
  order: ViewOrder;
  transaction: ViewPaymentTransaction;
}) {
  const router = useRouter();
  const [processing, setProcessing] = useState(false);
  const [resultMessage, setResultMessage] = useState("");
  const [status, setStatus] = useState<PaymentStatus>(transaction.status);

  async function handleSimulate(success: boolean) {
    setProcessing(true);
    setResultMessage("");
    try {
      const res = await processMockPaymentAction(transaction.id, success);
      if (res.ok) {
        setStatus(PaymentStatus.PAID);
        setResultMessage(
          "✓ Thanh toán giả lập thành công! Trạng thái đơn đã chuyển sang Đã thanh toán (PAID).",
        );
        setTimeout(() => {
          router.push("/account/orders?paid=1");
        }, 2000);
      } else {
        setStatus(PaymentStatus.FAILED);
        setResultMessage(res.message || "Thanh toán thất bại.");
      }
    } catch {
      setResultMessage("Có lỗi xảy ra khi xử lý thanh toán giả lập.");
    } finally {
      setProcessing(false);
    }
  }

  const isPaid = status === PaymentStatus.PAID;

  return (
    <div className="mx-auto max-w-lg rounded-2xl border bg-white p-6 shadow-sm">
      {/* Header */}
      <div className="text-center">
        <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-brand/10 text-2xl">
          {transaction.method === PaymentMethod.MOCK_MOMO
            ? "🟣"
            : transaction.method === PaymentMethod.MOCK_BANK
              ? "🏦"
              : "💳"}
        </div>
        <h1 className="mt-3 text-xl font-bold text-gray-900">
          Cổng thanh toán Demo (Mock Payment)
        </h1>
        <p className="mt-1 text-xs text-gray-500">
          {transaction.methodText} · Mã đơn: #{order.id.slice(0, 8)}
        </p>
      </div>

      {/* Amount Card */}
      <div className="mt-6 rounded-xl bg-gray-50 p-4 text-center">
        <span className="text-xs text-gray-500">Số tiền cần thanh toán</span>
        <div className="mt-1 text-2xl font-extrabold text-brand">
          {transaction.amount.toLocaleString("vi-VN")} {transaction.currency}
        </div>
      </div>

      {/* Mock Visual (QR Code / Bank Card) */}
      <div className="mt-6 rounded-xl border border-dashed border-gray-300 p-6 text-center">
        {transaction.method === PaymentMethod.MOCK_MOMO ? (
          <div className="space-y-3">
            <div className="mx-auto grid h-44 w-44 place-items-center rounded-lg bg-pink-50 p-3 ring-1 ring-pink-200">
              <div className="text-center">
                <span className="text-4xl">📱</span>
                <p className="mt-2 text-[11px] font-bold text-pink-700">
                  QR MOMO DEMO
                </p>
                <p className="text-[10px] text-gray-400">Quét để thanh toán</p>
              </div>
            </div>
            <p className="text-xs text-gray-500">
              Chủ tài khoản:{" "}
              <strong className="text-gray-800">CHỢ PLATFORM DEMO</strong>
            </p>
          </div>
        ) : transaction.method === PaymentMethod.MOCK_BANK ? (
          <div className="space-y-2 text-left text-xs text-gray-600">
            <div className="flex justify-between border-b pb-2">
              <span>Ngân hàng:</span>
              <strong className="text-gray-800">MB Bank (Demo)</strong>
            </div>
            <div className="flex justify-between border-b pb-2">
              <span>Số tài khoản:</span>
              <strong className="font-mono text-gray-800">
                9999 8888 7777
              </strong>
            </div>
            <div className="flex justify-between border-b pb-2">
              <span>Chủ tài khoản:</span>
              <strong className="text-gray-800">CHO PLATFORM DEMO</strong>
            </div>
            <div className="flex justify-between">
              <span>Nội dung CK:</span>
              <strong className="font-mono text-brand">
                DH {order.id.slice(0, 8)}
              </strong>
            </div>
          </div>
        ) : (
          <div className="space-y-2 rounded-lg bg-gradient-to-r from-blue-700 to-indigo-800 p-4 text-left text-white shadow-sm">
            <div className="text-xs font-semibold tracking-widest text-blue-200">
              CREDIT CARD DEMO
            </div>
            <div className="py-2 font-mono text-lg tracking-widest">
              •••• •••• •••• 8888
            </div>
            <div className="flex justify-between text-[11px] text-blue-200">
              <span>CHỦ THẺ: NGUYEN VAN A</span>
              <span>EXP: 12/28</span>
            </div>
          </div>
        )}
      </div>

      {resultMessage && (
        <div
          className={`mt-4 rounded-lg p-3 text-xs font-medium ${
            isPaid
              ? "bg-green-50 text-green-700 ring-1 ring-green-200"
              : "bg-red-50 text-red-700 ring-1 ring-red-200"
          }`}
        >
          {resultMessage}
        </div>
      )}

      {/* Simulator Action Buttons */}
      <div className="mt-6 space-y-2.5">
        {!isPaid ? (
          <>
            <button
              type="button"
              disabled={processing}
              onClick={() => handleSimulate(true)}
              className="w-full rounded-lg bg-emerald-600 py-3 text-sm font-semibold text-white shadow-sm transition hover:bg-emerald-700 disabled:opacity-60"
            >
              {processing
                ? "Đang xử lý..."
                : "✓ Xác nhận thanh toán (Simulate Success)"}
            </button>
            <button
              type="button"
              disabled={processing}
              onClick={() => handleSimulate(false)}
              className="w-full rounded-lg border border-red-200 bg-red-50 py-2.5 text-xs font-semibold text-red-600 hover:bg-red-100 disabled:opacity-60"
            >
              ✕ Giả lập thất bại (Simulate Failure)
            </button>
          </>
        ) : (
          <Link
            href="/account/orders"
            className="block w-full rounded-lg bg-brand py-3 text-center text-sm font-semibold text-white shadow-sm hover:bg-brand-dark"
          >
            Xem danh sách đơn hàng
          </Link>
        )}

        <div className="text-center">
          <Link
            href="/account/orders"
            className="text-xs text-gray-400 hover:text-gray-600"
          >
            ← Bỏ qua và thanh toán sau
          </Link>
        </div>
      </div>
    </div>
  );
}
