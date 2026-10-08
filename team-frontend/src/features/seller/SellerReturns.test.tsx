import { render, screen, waitFor, within } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";
import {
  PaymentRefundSource,
  PaymentStatus,
} from "@/generated/platform/payment/v1/payment_pb.js";
import type { ViewOrderReturn } from "@/lib/gateway/orders";
import type { ViewPaymentTransaction } from "@/lib/gateway/payment";
import { setupUser } from "@/test/user";

import { SellerReturns } from "./SellerReturns";
import {
  approveReturnAction,
  refundReturnAction,
  rejectReturnAction,
} from "./actions";
import { COD_RETURN_MESSAGE } from "./returnRefund";

vi.mock("./actions", () => ({
  approveReturnAction: vi.fn(),
  rejectReturnAction: vi.fn(),
  refundReturnAction: vi.fn(),
}));
const toast = { success: vi.fn(), error: vi.fn(), info: vi.fn() };
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));

const TEXT: Record<number, string> = {
  [ReturnStatus.PENDING]: "Chờ duyệt",
  [ReturnStatus.APPROVED]: "Đã duyệt",
  [ReturnStatus.REJECTED]: "Đã từ chối",
  [ReturnStatus.REFUNDED]: "Đã hoàn tiền",
};

function ret(status: ReturnStatus, over: Partial<ViewOrderReturn> = {}) {
  return {
    id: "r1",
    orderId: "o1",
    reason: "hỏng",
    refundAmount: 200000,
    status,
    statusText: TEXT[status],
    ...over,
  } satisfies ViewOrderReturn;
}

function payment(over: Partial<ViewPaymentTransaction> = {}) {
  return {
    amount: 500000,
    refundedAmount: 0,
    refunds: [],
    status: PaymentStatus.PAID,
    statusText: "Đã thanh toán",
    ...over,
  } as ViewPaymentTransaction;
}

function renderReturns(
  returns: ViewOrderReturn[],
  opts: { paidOnline?: boolean; payment?: ViewPaymentTransaction | null } = {},
) {
  return render(
    <SellerReturns
      orderId="o1"
      paidOnline={opts.paidOnline ?? true}
      returns={returns}
      payment={opts.payment === undefined ? payment() : opts.payment}
    />,
  );
}

const names = () =>
  screen
    .queryAllByRole("button")
    .map((b) => b.textContent)
    .filter((t) => ["Duyệt", "Từ chối", "Hoàn tiền"].includes(t ?? ""));

beforeEach(() => vi.clearAllMocks());

describe("SellerReturns actions per status", () => {
  it.each([
    [ReturnStatus.PENDING, ["Duyệt", "Từ chối"]],
    [ReturnStatus.APPROVED, ["Hoàn tiền", "Từ chối"]],
    [ReturnStatus.REJECTED, []],
    [ReturnStatus.REFUNDED, []],
  ])("status %s offers %j", (status, expected) => {
    renderReturns([ret(status)]);
    expect(names()).toEqual(expected);
  });

  it("approves from PENDING through the approve action only", async () => {
    vi.mocked(approveReturnAction).mockResolvedValue({ ok: true });
    renderReturns([ret(ReturnStatus.PENDING)]);
    await setupUser().click(screen.getByTestId("return-approve"));
    await waitFor(() =>
      expect(approveReturnAction).toHaveBeenCalledWith("r1", "o1"),
    );
    expect(refundReturnAction).not.toHaveBeenCalled();
    expect(toast.success).toHaveBeenCalled();
  });

  it("rejects a pending return", async () => {
    vi.mocked(rejectReturnAction).mockResolvedValue({ ok: true });
    renderReturns([ret(ReturnStatus.PENDING)]);
    await setupUser().click(screen.getByTestId("return-reject"));
    await waitFor(() =>
      expect(rejectReturnAction).toHaveBeenCalledWith("r1", "o1"),
    );
  });

  it("refunds an APPROVED return only after the confirmation", async () => {
    vi.mocked(refundReturnAction).mockResolvedValue({ ok: true });
    renderReturns([ret(ReturnStatus.APPROVED)]);
    const user = setupUser();
    await user.click(screen.getByTestId("return-refund"));
    expect(refundReturnAction).not.toHaveBeenCalled();
    await user.click(screen.getByTestId("return-refund-confirm"));
    await waitFor(() =>
      expect(refundReturnAction).toHaveBeenCalledWith("r1", "o1"),
    );
  });

  it("toasts the error when the refund is refused (FAILED_PRECONDITION)", async () => {
    vi.mocked(refundReturnAction).mockResolvedValue({
      ok: false,
      error: "return is not approved",
    });
    renderReturns([ret(ReturnStatus.APPROVED)]);
    const user = setupUser();
    await user.click(screen.getByTestId("return-refund"));
    await user.click(screen.getByTestId("return-refund-confirm"));
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("return is not approved"),
    );
  });

  it("shows the COD message and no Hoàn tiền for an unpaid-online order", () => {
    renderReturns([ret(ReturnStatus.APPROVED)], { paidOnline: false });
    expect(screen.getByTestId("return-cod-message")).toHaveTextContent(
      COD_RETURN_MESSAGE,
    );
    expect(names()).toEqual(["Từ chối"]);
  });

  it("does not show the COD message for a pending return", () => {
    renderReturns([ret(ReturnStatus.PENDING)], { paidOnline: false });
    expect(screen.queryByTestId("return-cod-message")).toBeNull();
    expect(names()).toEqual(["Duyệt", "Từ chối"]);
  });
});

describe("SellerReturns refund state", () => {
  const refundOf = (amount: number, requested: number) =>
    payment({
      refundedAmount: amount,
      refunds: [
        {
          id: "f1",
          source: PaymentRefundSource.RETURN,
          sourceId: "r1",
          requestedAmount: requested,
          amount,
          reason: "return_refunded",
        },
      ],
    });

  it("reads processing until the payment lists the refund", () => {
    renderReturns([ret(ReturnStatus.REFUNDED)]);
    expect(screen.getByTestId("return-refund-state")).toHaveTextContent(
      "Đang xử lý hoàn tiền",
    );
  });

  it("reads processing and says the payment is unavailable when it cannot be read", () => {
    renderReturns(
      [ret(ReturnStatus.APPROVED), ret(ReturnStatus.REFUNDED, { id: "r2" })],
      {
        payment: null,
      },
    );
    expect(screen.getByTestId("payment-unavailable")).toBeInTheDocument();
    expect(screen.queryByTestId("payment-summary")).toBeNull();
    expect(screen.getByTestId("return-refund-state")).toHaveTextContent(
      "Đang xử lý hoàn tiền",
    );
    // the returns and their actions still render
    expect(names()).toContain("Hoàn tiền");
  });

  it("reads the applied amount once the payment lists the refund", () => {
    renderReturns([ret(ReturnStatus.REFUNDED)], {
      payment: refundOf(200000, 200000),
    });
    expect(screen.getByTestId("return-refund-state")).toHaveTextContent(
      "Đã hoàn 200.000₫",
    );
    expect(
      within(screen.getByTestId("payment-summary")).getByText(
        "200.000₫ / 500.000₫",
      ),
    ).toBeInTheDocument();
  });

  it("says how much was refunded when the applied amount is lower", () => {
    renderReturns([ret(ReturnStatus.REFUNDED, { refundAmount: 300000 })], {
      payment: refundOf(100000, 300000),
    });
    expect(screen.getByTestId("return-refund-state")).toHaveTextContent(
      "Chỉ hoàn được 100.000₫",
    );
  });
});
