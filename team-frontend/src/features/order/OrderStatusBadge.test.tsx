import { render, screen } from "@testing-library/react";
import React from "react";
import { describe, expect, it } from "vitest";

import { toneStyles } from "@/components/ui/tones";
import {
  OrderStatus,
  ReturnStatus,
} from "@/generated/platform/order/v1/order_pb.js";

import { OrderStatusBadge, orderStatusTone } from "./OrderStatusBadge";

describe("OrderStatusBadge", () => {
  it("maps order statuses to tones", () => {
    expect(orderStatusTone({ status: OrderStatus.PENDING })).toBe("warning");
    expect(orderStatusTone({ status: OrderStatus.PAID })).toBe("info");
    expect(orderStatusTone({ status: OrderStatus.SHIPPED })).toBe("info");
    expect(orderStatusTone({ status: OrderStatus.COMPLETED })).toBe("success");
    expect(orderStatusTone({ status: OrderStatus.CANCELLED })).toBe("neutral");
  });

  it("maps return statuses to tones", () => {
    expect(
      orderStatusTone({ kind: "return", status: ReturnStatus.PENDING }),
    ).toBe("warning");
    expect(
      orderStatusTone({ kind: "return", status: ReturnStatus.APPROVED }),
    ).toBe("info");
    expect(
      orderStatusTone({ kind: "return", status: ReturnStatus.REJECTED }),
    ).toBe("danger");
    expect(
      orderStatusTone({ kind: "return", status: ReturnStatus.REFUNDED }),
    ).toBe("success");
  });

  it("renders the label with the tone classes, no emoji and no brand colour", () => {
    render(
      <OrderStatusBadge
        status={OrderStatus.COMPLETED}
        label="Đã hoàn thành"
        data-testid="badge"
      />,
    );
    const badge = screen.getByTestId("badge");
    expect(badge).toHaveTextContent("Đã hoàn thành");
    for (const cls of toneStyles.success.soft.split(" ")) {
      expect(badge).toHaveClass(cls);
    }
    expect(badge.textContent).toMatch(/^[\p{L}\s]+$/u);
    expect(badge.className).not.toMatch(/action-primary|brand/);
  });

  it("keeps a testid on return badges", () => {
    render(
      <OrderStatusBadge
        kind="return"
        status={ReturnStatus.REFUNDED}
        label="Đã hoàn tiền"
        data-testid="return-status"
      />,
    );
    expect(screen.getByTestId("return-status")).toHaveTextContent(
      "Đã hoàn tiền",
    );
  });

  it("renders an unknown status as a neutral tag with its text", () => {
    render(
      <OrderStatusBadge
        status={99 as OrderStatus}
        label="Lạ"
        data-testid="badge"
      />,
    );
    const badge = screen.getByTestId("badge");
    expect(badge).toHaveTextContent("Lạ");
    for (const cls of toneStyles.neutral.soft.split(" ")) {
      expect(badge).toHaveClass(cls);
    }
  });
});
