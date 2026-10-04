import React from "react";

import { Card } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";
import { getSagaState, getShipmentTracking } from "@/lib/gateway/orders";
import { OrderTimeline } from "./OrderTimeline";

/** Fetches shipment and saga in parallel, then renders the timeline. */
export async function OrderTimelineSection({ orderId }: { orderId: string }) {
  const [shipment, sagaSteps] = await Promise.all([
    getShipmentTracking(orderId),
    getSagaState(orderId),
  ]);
  return (
    <OrderTimeline
      orderId={orderId}
      shipment={shipment}
      sagaSteps={sagaSteps}
    />
  );
}

/** Suspense fallback with the footprint of the timeline card. */
export function OrderTimelineSkeleton() {
  return (
    <Card className="min-h-48 p-5">
      <Skeleton variant="text" lines={5} />
    </Card>
  );
}
