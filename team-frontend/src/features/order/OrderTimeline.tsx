import React from "react";

import { Alert } from "@/components/ui/Alert";
import { Empty } from "@/components/ui/Empty";
import { Timeline, type TimelineItem } from "@/components/ui/Timeline";
import type { Tone } from "@/components/ui/tones";
import type { ViewSagaStep, ViewShipment } from "@/lib/gateway/orders";
import { ReorderButton } from "./OrderActions";

function sagaTone(status: string): Tone {
  switch (status.toUpperCase()) {
    case "SUCCESS":
      return "success";
    case "FAILED":
    case "COMPENSATED":
      return "danger";
    case "PENDING":
      return "warning";
    default:
      return "neutral";
  }
}

function isFailure(step: ViewSagaStep): boolean {
  const s = step.status.toUpperCase();
  return s === "FAILED" || s === "COMPENSATED";
}

function checkpointItems(shipment: ViewShipment): TimelineItem[] {
  // The gateway returns checkpoints oldest first; show the newest on top.
  return [...shipment.checkpoints].reverse().map((c, i) => ({
    key: `${c.timestamp}-${i}`,
    testId: "timeline-checkpoint",
    title: c.description || "Cập nhật",
    description: [c.location, c.timestamp].filter(Boolean).join(" · "),
    tone: i === 0 ? "primary" : "neutral",
    current: i === 0,
  }));
}

function sagaItems(steps: ViewSagaStep[]): TimelineItem[] {
  return steps.map((s, i) => {
    const failed = isFailure(s);
    return {
      key: `${s.name}-${i}`,
      testId: "timeline-saga-step",
      tone: sagaTone(s.status),
      title: failed ? (
        <span data-testid="timeline-failure" className="block">
          <span className="block">{s.name}</span>
          {s.detail && (
            <span className="block font-normal text-danger">{s.detail}</span>
          )}
          {s.timestamp && (
            <span className="block font-normal text-text-disabled">
              {s.timestamp}
            </span>
          )}
        </span>
      ) : (
        s.name
      ),
      description: failed
        ? undefined
        : [s.detail, s.timestamp].filter(Boolean).join(" · "),
    };
  });
}

/**
 * Order timeline on `Timeline`: shipment checkpoints (newest first), else the
 * saga steps with an explicit failure checkpoint, else Empty. Server-compatible.
 * Pass `orderId` to offer "Mua lại" in the failure alert.
 */
export function OrderTimeline({
  shipment,
  sagaSteps,
  orderId,
}: {
  shipment: ViewShipment | null;
  sagaSteps: ViewSagaStep[];
  orderId?: string;
}) {
  const hasCheckpoints = (shipment?.checkpoints.length ?? 0) > 0;
  const hasSaga = sagaSteps.length > 0;
  const failure = !hasCheckpoints
    ? sagaSteps.find((s) => isFailure(s) && s.detail) ||
      sagaSteps.find(isFailure)
    : undefined;

  return (
    <div
      data-testid="order-timeline"
      className="rounded-xl border border-border-subtle bg-surface-card p-5"
    >
      <div className="border-b border-border-subtle pb-4">
        <h2 className="text-base font-semibold text-text-primary">
          Hành trình đơn hàng
        </h2>
        {shipment && (
          <p className="mt-1 text-xs text-text-secondary">
            {shipment.carrier || "Đơn vị vận chuyển"} · Mã vận đơn:{" "}
            <span className="font-semibold text-text-primary">
              {shipment.trackingCode || "—"}
            </span>{" "}
            ·{" "}
            <span className="font-medium text-text-primary">
              {shipment.statusText}
            </span>
          </p>
        )}
      </div>

      {failure && (
        <Alert
          type="error"
          className="mt-4"
          title="Đơn hàng không hoàn tất"
          description={
            failure.detail ||
            "Thanh toán hoặc giữ hàng thất bại, đơn đã được hoàn tác."
          }
          action={orderId ? <ReorderButton orderId={orderId} /> : undefined}
        />
      )}

      <div className="mt-5">
        {!hasCheckpoints && !hasSaga ? (
          <div data-testid="timeline-empty">
            <Empty description="Chưa có thông tin vận chuyển cho đơn hàng này." />
          </div>
        ) : hasCheckpoints && shipment ? (
          <Timeline items={checkpointItems(shipment)} />
        ) : (
          <div data-testid="timeline-saga">
            <Timeline items={sagaItems(sagaSteps)} />
          </div>
        )}
      </div>
    </div>
  );
}
