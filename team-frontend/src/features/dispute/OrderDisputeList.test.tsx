import { render, screen, within } from "@testing-library/react";
import React from "react";
import { describe, expect, it } from "vitest";

import type { ViewDispute } from "@/lib/gateway/disputes";

import { OrderDisputeList } from "./OrderDisputeList";

// Contract under test (implementer creates OrderDisputeList.tsx):
//   <OrderDisputeList disputes={ViewDispute[]} failed?={boolean} />
//   section heading "Khiếu nại"; one data-testid="dispute-item" per dispute in
//   the given order, showing statusText, rulingText (when set), resolution and
//   reason as plain text; empty => "Chưa có khiếu nại";
//   failed => "Không tải được danh sách khiếu nại." and no items.

const base: ViewDispute = {
  id: "d1",
  orderId: "o1",
  claimantId: "b1",
  defendantId: "s1",
  reason: "Người bán từ chối vô lý",
  resolution: "",
  returnId: "r1",
  evidenceUrls: [],
  status: "OPEN",
  statusText: "Đang chờ xử lý",
  ruling: "",
  rulingText: "",
  createdAt: "2026-10-22T00:00:00.000Z",
};

describe("OrderDisputeList", () => {
  it("[Disputes are listed with status and ruling] lists disputes with status and ruling in server order", () => {
    render(
      <OrderDisputeList
        disputes={[
          {
            ...base,
            id: "d2",
            status: "RESOLVED",
            statusText: "Đã giải quyết",
            ruling: "BUYER_FAVOUR",
            rulingText: "Người mua thắng",
            resolution: "Hoàn tiền",
          },
          {
            ...base,
            id: "d1",
            status: "REJECTED",
            statusText: "Đã từ chối",
          },
        ]}
      />,
    );
    const items = screen.getAllByTestId("dispute-item");
    expect(items).toHaveLength(2);
    expect(within(items[0]).getByText("Đã giải quyết")).toBeInTheDocument();
    expect(within(items[0]).getByText("Người mua thắng")).toBeInTheDocument();
    expect(within(items[0]).getByText("Hoàn tiền")).toBeInTheDocument();
    expect(within(items[1]).getByText("Đã từ chối")).toBeInTheDocument();
    expect(within(items[1]).queryByText("Người mua thắng")).toBeNull();
    expect(within(items[1]).queryByText("Người bán thắng")).toBeNull();
  });

  it("[Order has no disputes] shows the empty state", () => {
    render(<OrderDisputeList disputes={[]} />);
    expect(screen.getByText("Chưa có khiếu nại")).toBeInTheDocument();
    expect(screen.queryAllByTestId("dispute-item")).toHaveLength(0);
  });

  it("[Dispute list failure leaves the order page intact] shows only the error text when the load failed", () => {
    render(<OrderDisputeList disputes={[]} failed />);
    expect(
      screen.getByText("Không tải được danh sách khiếu nại."),
    ).toBeInTheDocument();
    expect(screen.queryByText("Chưa có khiếu nại")).toBeNull();
    expect(screen.queryAllByTestId("dispute-item")).toHaveLength(0);
  });

  it("[Markup in a dispute is shown as text] renders dispute text as text", () => {
    const markup = "<img src=x onerror=alert(1)>";
    const { container } = render(
      <OrderDisputeList
        disputes={[{ ...base, reason: markup, resolution: markup }]}
      />,
    );
    expect(screen.getAllByText(markup).length).toBeGreaterThan(0);
    expect(container.querySelector("img")).toBeNull();
  });
});
