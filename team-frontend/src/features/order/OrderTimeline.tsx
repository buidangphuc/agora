import React from "react";

import type { ViewSagaStep, ViewShipment } from "@/lib/gateway/orders";

function sagaDotClass(status: string): string {
  switch (status.toUpperCase()) {
    case "SUCCESS":
      return "bg-emerald-500 ring-4 ring-emerald-50";
    case "FAILED":
    case "COMPENSATED":
      return "bg-rose-500 ring-4 ring-rose-50";
    case "PENDING":
      return "bg-amber-400 ring-4 ring-amber-50";
    default:
      return "bg-gray-300 ring-4 ring-gray-100";
  }
}

/**
 * Preline-styled order timeline.
 * Shows delivery checkpoints (real progress) or falls back to saga steps.
 */
export function OrderTimeline({
  shipment,
  sagaSteps,
}: {
  shipment: ViewShipment | null;
  sagaSteps: ViewSagaStep[];
}) {
  const checkpoints = shipment?.checkpoints ?? [];
  const hasCheckpoints = checkpoints.length > 0;
  const hasSaga = sagaSteps.length > 0;

  return (
    <div
      data-testid="order-timeline"
      className="mt-6 rounded-xl border border-gray-200/90 bg-white p-6 shadow-preline-card"
    >
      <div className="border-b border-gray-100 pb-4">
        <h2 className="text-base font-bold text-gray-900 tracking-tight">
          HÀNH TRÌNH ĐƠN HÀNG
        </h2>
        {shipment && (
          <p className="mt-1 text-xs text-gray-500">
            {shipment.carrier || "Đơn vị vận chuyển"} · Mã vận đơn:{" "}
            <span className="font-semibold text-gray-700">
              {shipment.trackingCode || "—"}
            </span>{" "}
            ·{" "}
            <span className="text-primary-600 font-medium">
              {shipment.statusText}
            </span>
          </p>
        )}
      </div>

      {!hasCheckpoints && !hasSaga ? (
        <div
          data-testid="timeline-empty"
          className="py-10 text-center text-xs text-gray-400"
        >
          Chưa có thông tin vận chuyển cho đơn hàng này.
        </div>
      ) : hasCheckpoints ? (
        <ol className="mt-6 space-y-4">
          {checkpoints.map((c, i) => (
            <li
              key={`${c.timestamp}-${i}`}
              data-testid="timeline-checkpoint"
              className="relative flex gap-4"
            >
              <div className="relative flex flex-col items-center">
                <span
                  className={`h-3 w-3 rounded-full transition ${
                    i === 0
                      ? "bg-primary-500 ring-4 ring-primary-100"
                      : "bg-gray-300 ring-4 ring-gray-100"
                  }`}
                />
                {i < checkpoints.length - 1 && (
                  <span className="mt-1 h-full w-0.5 flex-1 bg-gray-200" />
                )}
              </div>
              <div className="pb-3">
                <p className="text-xs font-semibold text-gray-800">
                  {c.description || "Cập nhật"}
                </p>
                <p className="text-[11px] text-gray-500 mt-0.5">
                  {[c.location, c.timestamp].filter(Boolean).join(" · ")}
                </p>
              </div>
            </li>
          ))}
        </ol>
      ) : (
        <ol data-testid="timeline-saga" className="mt-6 space-y-4">
          {sagaSteps.map((s, i) => (
            <li
              key={`${s.name}-${i}`}
              data-testid="timeline-saga-step"
              className="relative flex gap-4"
            >
              <div className="relative flex flex-col items-center">
                <span
                  className={`h-3 w-3 rounded-full ${sagaDotClass(s.status)}`}
                />
                {i < sagaSteps.length - 1 && (
                  <span className="mt-1 h-full w-0.5 flex-1 bg-gray-200" />
                )}
              </div>
              <div className="pb-3">
                <p className="text-xs font-semibold text-gray-800">{s.name}</p>
                <p className="text-[11px] text-gray-500 mt-0.5">
                  {[s.detail, s.timestamp].filter(Boolean).join(" · ")}
                </p>
              </div>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}
