import { render, screen } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

import { ShipmentStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewShipment } from "@/lib/gateway/orders";

import { OrderTimeline } from "./OrderTimeline";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));
vi.mock("./actions", () => ({
  reorderAction: vi.fn(),
  cancelOrderAction: vi.fn(),
}));

const shipmentWithCheckpoints: ViewShipment = {
  id: "s1",
  carrier: "GHN",
  trackingCode: "TRK123",
  status: ShipmentStatus.IN_TRANSIT,
  statusText: "Đang vận chuyển",
  checkpoints: [
    {
      timestamp: "01/09 10:00",
      location: "Hà Nội",
      description: "Đã lấy hàng",
    },
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

  it("lists checkpoints newest first and marks the newest as current", () => {
    render(<OrderTimeline shipment={shipmentWithCheckpoints} sagaSteps={[]} />);
    const items = screen.getAllByTestId("timeline-checkpoint");
    expect(items[0]).toHaveTextContent("Đang giao");
    expect(items[0]).toHaveAttribute("aria-current", "step");
    expect(items[1]).toHaveTextContent("Đã lấy hàng");
    expect(items[1]).not.toHaveAttribute("aria-current");
    expect(screen.getByText("GHN", { exact: false })).toBeInTheDocument();
    expect(screen.getByText("Đang vận chuyển")).toBeInTheDocument();
  });

  it("surfaces a failed saga as the failure checkpoint with a recovery action", () => {
    render(
      <OrderTimeline
        orderId="o1"
        shipment={null}
        sagaSteps={[
          {
            name: "1. Order Created",
            status: "SUCCESS",
            detail: "",
            timestamp: "01/09",
          },
          {
            name: "2. Stock Reserved",
            status: "SUCCESS",
            detail: "",
            timestamp: "01/09",
          },
          {
            name: "3. Payment",
            status: "FAILED",
            detail: "Thẻ bị từ chối",
            timestamp: "01/09",
          },
          {
            name: "4. Stock Released",
            status: "COMPENSATED",
            detail: "Đã hoàn kho",
            timestamp: "01/09",
          },
        ]}
      />,
    );
    expect(screen.getAllByTestId("timeline-saga-step")).toHaveLength(4);
    const failures = screen.getAllByTestId("timeline-failure");
    expect(failures).toHaveLength(2);
    expect(failures[0]).toHaveTextContent("Thẻ bị từ chối");
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Thẻ bị từ chối");
    expect(screen.getByRole("button", { name: "Mua lại" })).toBeInTheDocument();
  });

  it("shows a pending saga step without a failure item or alert", () => {
    render(
      <OrderTimeline
        orderId="o1"
        shipment={null}
        sagaSteps={[
          {
            name: "1. Order Created",
            status: "SUCCESS",
            detail: "",
            timestamp: "01/09",
          },
          {
            name: "3. Payment",
            status: "PENDING",
            detail: "Đang chờ",
            timestamp: "",
          },
        ]}
      />,
    );
    expect(screen.getAllByTestId("timeline-saga-step")).toHaveLength(2);
    expect(screen.queryByTestId("timeline-failure")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("omits the recovery action when no order id is given", () => {
    render(
      <OrderTimeline
        shipment={null}
        sagaSteps={[
          { name: "3. Payment", status: "FAILED", detail: "x", timestamp: "" },
        ]}
      />,
    );
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Mua lại" })).toBeNull();
  });
});
