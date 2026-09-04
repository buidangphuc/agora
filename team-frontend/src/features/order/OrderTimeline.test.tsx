import { render, screen } from "@testing-library/react";
import React from "react";
import { describe, expect, it } from "vitest";

import { ShipmentStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewShipment } from "@/lib/gateway/orders";

import { OrderTimeline } from "./OrderTimeline";

const shipmentWithCheckpoints: ViewShipment = {
  id: "s1",
  carrier: "GHN",
  trackingCode: "TRK123",
  status: ShipmentStatus.IN_TRANSIT,
  statusText: "Đang vận chuyển",
  checkpoints: [
    { timestamp: "01/09 10:00", location: "Hà Nội", description: "Đã lấy hàng" },
    { timestamp: "02/09 08:00", location: "Đà Nẵng", description: "Đang giao" },
  ],
};

describe("OrderTimeline", () => {
  it("renders carrier checkpoints when a shipment has them", () => {
    render(<OrderTimeline shipment={shipmentWithCheckpoints} sagaSteps={[]} />);
    expect(screen.getAllByTestId("timeline-checkpoint")).toHaveLength(2);
    expect(screen.getByText("Đã lấy hàng")).toBeInTheDocument();
    expect(screen.getByText(/TRK123/)).toBeInTheDocument();
  });

  it("falls back to saga steps when there are no checkpoints", () => {
    render(
      <OrderTimeline
        shipment={null}
        sagaSteps={[
          {
            name: "2. Stock Reserved",
            status: "SUCCESS",
            detail: "ok",
            timestamp: "01/09",
          },
        ]}
      />,
    );
    expect(screen.getByTestId("timeline-saga")).toBeInTheDocument();
    expect(screen.getByText("2. Stock Reserved")).toBeInTheDocument();
    expect(screen.queryByTestId("timeline-checkpoint")).toBeNull();
  });

  it("shows an empty state when there is neither shipment nor saga", () => {
    render(<OrderTimeline shipment={null} sagaSteps={[]} />);
    expect(screen.getByTestId("timeline-empty")).toBeInTheDocument();
  });
});
