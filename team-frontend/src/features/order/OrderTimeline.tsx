import React from "react";

import type { ViewSagaStep, ViewShipment } from "@/lib/gateway/orders";

function sagaDotClass(status: string): string {
  switch (status.toUpperCase()) {
    case "SUCCESS":
      return "bg-emerald-500";
    case "FAILED":
    case "COMPENSATED":
      return "bg-red-500";
    case "PENDING":
      return "bg-amber-400";
    default:
      return "bg-gray-300";
  }
}

/**
 * Minimal post-purchase timeline. Prefers carrier checkpoints (real delivery
 * progress); falls back to the order saga steps (order → stock → payment →
 * confirm) when no shipment exists yet. Loading/empty handled by the caller
 * passing null/[].
 */
export function OrderTimeline({
  shipment,
  sagaSteps,
}: {
  shipment: ViewShipment | null;
  sagaSteps: ViewSagaStep[];
}) {
  const hasCheckpoints = !!shipment && shipment.checkpoints.length > 0;
  const hasSaga = sagaSteps.length > 0;

  return (
    <div
      data-testid="order-timeline"
      className="mt-6 rounded-2xl border bg-white p-6 shadow-xs"
    >
      <div className="border-b pb-4">
        <h2 className="text-lg font-bold text-gray-900">HÀNH TRÌNH ĐƠN HÀNG</h2>
        {shipment && (
          <p className="mt-0.5 text-xs text-gray-500">
            {shipment.carrier || "Đơn vị vận chuyển"} · Mã vận đơn:{" "}
            <span className="font-semibold text-gray-700">
              {shipment.trackingCode || "—"}
            </span>{" "}
            · {shipment.statusText}
          </p>
        )}
      </div>

      {!hasCheckpoints && !hasSaga ? (
        <div
          data-testid="timeline-empty"
          className="py-8 text-center text-xs text-gray-400"
        >
          Chưa có thông tin vận chuyển cho đơn hàng này.
        </div>
      ) : hasCheckpoints ? (
        <ol className="mt-5 space-y-4">
          {shipment!.checkpoints.map((c, i) => (
            <li
              key={`${c.timestamp}-${i}`}
              data-testid="timeline-checkpoint"
              className="flex gap-3"
            >
              <div className="mt-1 flex flex-col items-center">
                <span
                  className={`h-2.5 w-2.5 rounded-full ${
                    i === 0 ? "bg-brand" : "bg-gray-300"
                  }`}
                />
                {i < shipment!.checkpoints.length - 1 && (
                  <span className="mt-1 h-full w-px flex-1 bg-gray-200" />
                )}
              </div>
              <div className="pb-1">
                <p className="text-xs font-semibold text-gray-800">
                  {c.description || "Cập nhật"}
                </p>
                <p className="text-[11px] text-gray-500">
                  {[c.location, c.timestamp].filter(Boolean).join(" · ")}
                </p>
              </div>
            </li>
          ))}
        </ol>
      ) : (
        <ol data-testid="timeline-saga" className="mt-5 space-y-4">
          {sagaSteps.map((s, i) => (
            <li
              key={`${s.name}-${i}`}
              data-testid="timeline-saga-step"
              className="flex gap-3"
            >
              <div className="mt-1 flex flex-col items-center">
                <span
                  className={`h-2.5 w-2.5 rounded-full ${sagaDotClass(s.status)}`}
                />
                {i < sagaSteps.length - 1 && (
                  <span className="mt-1 h-full w-px flex-1 bg-gray-200" />
                )}
              </div>
              <div className="pb-1">
                <p className="text-xs font-semibold text-gray-800">{s.name}</p>
                <p className="text-[11px] text-gray-500">
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
