"use client";

import React, { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewOrderReturn } from "@/lib/gateway/orders";
import { createReturnRequestAction, mockRefundAction } from "./actions";

function StatusBadge({ ret }: { ret: ViewOrderReturn }) {
  const tone =
    ret.status === ReturnStatus.REFUNDED
      ? "bg-emerald-50 text-emerald-700"
      : ret.status === ReturnStatus.REJECTED
        ? "bg-red-50 text-red-700"
        : "bg-amber-50 text-amber-700";
  return (
    <span
      data-testid="return-status"
      className={`inline-flex rounded-2xs px-2 py-0.5 text-[11px] font-semibold ${tone}`}
    >
      {ret.statusText}
    </span>
  );
}

export function ReturnRequestSection({
  orderId,
  orderTotal,
  initialReturn = null,
}: {
  orderId: string;
  orderTotal: number;
  initialReturn?: ViewOrderReturn | null;
}) {
  const toast = useToast();
  const [ret, setRet] = useState<ViewOrderReturn | null>(initialReturn);
  const [reason, setReason] = useState("");
  const [amount, setAmount] = useState<number>(orderTotal);
  const [pending, start] = useTransition();

  function submit() {
    if (pending) return;
    start(async () => {
      const res = await createReturnRequestAction(orderId, reason, amount);
      if (res.ok && res.returnRequest) {
        setRet(res.returnRequest);
        toast.success(res.message || "Đã gửi yêu cầu trả hàng.");
      } else {
        toast.error(res.message || "Gửi yêu cầu thất bại.");
      }
    });
  }

  function refund() {
    if (pending || !ret) return;
    start(async () => {
      const res = await mockRefundAction(ret.id, orderId, ret.refundAmount);
      if (res.ok && res.returnRequest) {
        setRet(res.returnRequest);
        toast.success(res.message || "Hoàn tiền thành công.");
      } else {
        toast.error(res.message || "Hoàn tiền thất bại.");
      }
    });
  }

  const canRefund =
    !!ret &&
    ret.status !== ReturnStatus.REFUNDED &&
    ret.status !== ReturnStatus.REJECTED;

  return (
    <div
      data-testid="return-section"
      className="mt-6 rounded-2xl border bg-white p-6 shadow-xs"
    >
      <div className="border-b pb-4">
        <h2 className="text-lg font-bold text-gray-900">TRẢ HÀNG / HOÀN TIỀN</h2>
        <p className="mt-0.5 text-xs text-gray-500">
          Gửi yêu cầu trả hàng cho đơn này. Hoàn tiền là mô phỏng (demo).
        </p>
      </div>

      {ret ? (
        <div className="mt-5 space-y-3">
          <div className="flex items-center gap-2">
            <span className="text-xs text-gray-500">Trạng thái:</span>
            <StatusBadge ret={ret} />
          </div>
          <p className="text-xs text-gray-700">
            <span className="text-gray-500">Lý do:</span> {ret.reason}
          </p>
          <p className="text-xs text-gray-700">
            <span className="text-gray-500">Số tiền hoàn:</span>{" "}
            {ret.refundAmount.toLocaleString("vi-VN")}₫
          </p>
          {canRefund && (
            <button
              type="button"
              data-testid="return-refund"
              onClick={refund}
              disabled={pending}
              className="rounded-lg bg-orange-50 px-4 py-2 text-xs font-semibold text-brand transition hover:bg-orange-100 disabled:opacity-60"
            >
              {pending ? "Đang xử lý…" : "Hoàn tiền (mô phỏng)"}
            </button>
          )}
        </div>
      ) : (
        <div className="mt-5 space-y-3">
          <div>
            <label
              htmlFor="return-reason"
              className="mb-1 block text-xs font-medium text-gray-600"
            >
              Lý do trả hàng
            </label>
            <textarea
              id="return-reason"
              data-testid="return-reason"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              rows={3}
              className="w-full rounded-lg border px-3 py-2 text-xs text-gray-800 focus:border-brand focus:outline-none"
              placeholder="Sản phẩm bị lỗi, giao sai mẫu…"
            />
          </div>
          <div>
            <label
              htmlFor="return-amount"
              className="mb-1 block text-xs font-medium text-gray-600"
            >
              Số tiền hoàn (₫)
            </label>
            <input
              id="return-amount"
              data-testid="return-amount"
              type="number"
              min={0}
              value={amount}
              onChange={(e) => setAmount(Number(e.target.value))}
              className="w-full rounded-lg border px-3 py-2 text-xs text-gray-800 focus:border-brand focus:outline-none"
            />
          </div>
          <button
            type="button"
            data-testid="return-submit"
            onClick={submit}
            disabled={pending}
            className="rounded-lg bg-brand px-4 py-2 text-xs font-semibold text-white transition hover:bg-orange-600 disabled:opacity-60"
          >
            {pending ? "Đang gửi…" : "Gửi yêu cầu trả hàng"}
          </button>
        </div>
      )}
    </div>
  );
}
