import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";

import { ReturnRequestSection } from "./ReturnRequestSection";
import { createReturnRequestAction, mockRefundAction } from "./actions";

vi.mock("./actions", () => ({
  createReturnRequestAction: vi.fn(),
  mockRefundAction: vi.fn(),
}));

const toast = { success: vi.fn(), error: vi.fn(), info: vi.fn() };
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => toast,
}));

beforeEach(() => vi.clearAllMocks());

describe("ReturnRequestSection", () => {
  it("submits a return request and then shows its status", async () => {
    vi.mocked(createReturnRequestAction).mockResolvedValue({
      ok: true,
      message: "Đã gửi",
      returnRequest: {
        id: "r1",
        orderId: "o1",
        reason: "hỏng",
        refundAmount: 50000,
        status: ReturnStatus.PENDING,
        statusText: "Chờ duyệt",
      },
    });

    render(<ReturnRequestSection orderId="o1" orderTotal={50000} />);
    const user = userEvent.setup();
    await user.type(screen.getByTestId("return-reason"), "hỏng");
    await user.click(screen.getByTestId("return-submit"));

    await waitFor(() =>
      expect(createReturnRequestAction).toHaveBeenCalledWith("o1", "hỏng", 50000),
    );
    expect(await screen.findByTestId("return-status")).toHaveTextContent(
      "Chờ duyệt",
    );
    expect(toast.success).toHaveBeenCalled();
  });

  it("renders an existing return and can run the mock refund", async () => {
    vi.mocked(mockRefundAction).mockResolvedValue({
      ok: true,
      returnRequest: {
        id: "r1",
        orderId: "o1",
        reason: "hỏng",
        refundAmount: 50000,
        status: ReturnStatus.REFUNDED,
        statusText: "Đã hoàn tiền",
      },
    });

    render(
      <ReturnRequestSection
        orderId="o1"
        orderTotal={50000}
        initialReturn={{
          id: "r1",
          orderId: "o1",
          reason: "hỏng",
          refundAmount: 50000,
          status: ReturnStatus.PENDING,
          statusText: "Chờ duyệt",
        }}
      />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByTestId("return-refund"));

    await waitFor(() =>
      expect(mockRefundAction).toHaveBeenCalledWith("r1", "o1", 50000),
    );
    expect(await screen.findByTestId("return-status")).toHaveTextContent(
      "Đã hoàn tiền",
    );
  });
});
