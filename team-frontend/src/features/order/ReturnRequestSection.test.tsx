import { render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";
import { setupUser } from "@/test/user";

import { ReturnRequestSection } from "./ReturnRequestSection";
import { ReturnStateProvider } from "./ReturnState";
import { createReturnRequestAction } from "./actions";

vi.mock("./actions", () => ({
  createReturnRequestAction: vi.fn(),
}));

const toast = { success: vi.fn(), error: vi.fn(), info: vi.fn() };
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => toast,
}));

const pendingReturn = {
  id: "r1",
  orderId: "o1",
  reason: "hỏng",
  refundAmount: 50000,
  status: ReturnStatus.PENDING,
  statusText: "Chờ duyệt",
};

beforeEach(() => vi.clearAllMocks());

async function openModal(user: ReturnType<typeof setupUser>) {
  await user.click(screen.getByRole("button", { name: "Yêu cầu trả hàng" }));
}

describe("ReturnRequestSection", () => {
  it("shows Empty without a request action when the order is not eligible", () => {
    render(<ReturnRequestSection orderId="o1" orderTotal={50000} />);
    expect(screen.getByTestId("return-section")).toBeInTheDocument();
    expect(screen.getByText("Chưa có yêu cầu trả hàng")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Yêu cầu trả hàng" }),
    ).toBeNull();
  });

  it("submits a return request from the Modal and then shows its status", async () => {
    vi.mocked(createReturnRequestAction).mockResolvedValue({
      ok: true,
      data: pendingReturn,
    });

    render(<ReturnRequestSection orderId="o1" orderTotal={50000} canRequest />);
    const user = setupUser();
    await openModal(user);
    await user.selectOptions(screen.getByTestId("return-reason"), "defective");
    await user.click(screen.getByTestId("return-submit"));

    await waitFor(() =>
      expect(createReturnRequestAction).toHaveBeenCalledWith(
        "o1",
        "Sản phẩm bị lỗi / hư hỏng",
        50000,
      ),
    );
    expect(await screen.findByTestId("return-status")).toHaveTextContent(
      "Chờ duyệt",
    );
    expect(toast.success).toHaveBeenCalled();
    expect(screen.queryByTestId("return-submit")).toBeNull();
  });

  it("blocks an invalid form with inline errors and calls no action", async () => {
    render(<ReturnRequestSection orderId="o1" orderTotal={50000} canRequest />);
    const user = setupUser();
    await openModal(user);
    await user.click(screen.getByTestId("return-submit"));
    expect(
      screen.getByText("Vui lòng chọn lý do trả hàng."),
    ).toBeInTheDocument();

    await user.selectOptions(
      screen.getByTestId("return-reason"),
      "changed_mind",
    );
    await user.clear(screen.getByTestId("return-amount"));
    await user.type(screen.getByTestId("return-amount"), "60000");
    await user.click(screen.getByTestId("return-submit"));
    expect(
      screen.getByText("Số tiền hoàn không được vượt quá tổng đơn hàng."),
    ).toBeInTheDocument();
    expect(createReturnRequestAction).not.toHaveBeenCalled();
    expect(screen.getByTestId("return-submit")).toBeInTheDocument();
  });

  it("requires a description for 'other'", async () => {
    render(<ReturnRequestSection orderId="o1" orderTotal={50000} canRequest />);
    const user = setupUser();
    await openModal(user);
    await user.selectOptions(screen.getByTestId("return-reason"), "other");
    await user.click(screen.getByTestId("return-submit"));
    expect(screen.getByText("Vui lòng mô tả lý do.")).toBeInTheDocument();
    await user.type(screen.getByTestId("return-reason-detail"), "hỏng");
    vi.mocked(createReturnRequestAction).mockResolvedValue({
      ok: true,
      data: pendingReturn,
    });
    await user.click(screen.getByTestId("return-submit"));
    await waitFor(() =>
      expect(createReturnRequestAction).toHaveBeenCalledWith(
        "o1",
        "hỏng",
        50000,
      ),
    );
  });

  it("keeps the Modal open and toasts the error when the action fails", async () => {
    vi.mocked(createReturnRequestAction).mockResolvedValue({
      ok: false,
      error: "boom",
    });
    render(<ReturnRequestSection orderId="o1" orderTotal={50000} canRequest />);
    const user = setupUser();
    await openModal(user);
    await user.selectOptions(
      screen.getByTestId("return-reason"),
      "changed_mind",
    );
    await user.click(screen.getByTestId("return-submit"));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("boom"));
    expect(screen.getByTestId("return-submit")).toBeEnabled();
    expect(screen.queryByTestId("return-status")).toBeNull();
  });

  it("disables the form and the submit button while pending", async () => {
    let resolve: (
      v: Awaited<ReturnType<typeof createReturnRequestAction>>,
    ) => void = () => {};
    vi.mocked(createReturnRequestAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );
    render(<ReturnRequestSection orderId="o1" orderTotal={50000} canRequest />);
    const user = setupUser();
    await openModal(user);
    await user.selectOptions(
      screen.getByTestId("return-reason"),
      "changed_mind",
    );
    await user.click(screen.getByTestId("return-submit"));

    await waitFor(() =>
      expect(screen.getByTestId("return-submit")).toBeDisabled(),
    );
    expect(screen.getByTestId("return-reason")).toBeDisabled();
    expect(screen.getByTestId("return-amount")).toBeDisabled();

    resolve({ ok: true, data: pendingReturn });
    expect(await screen.findByTestId("return-status")).toBeInTheDocument();
  });

  it.each([
    [ReturnStatus.PENDING, "Chờ duyệt"],
    [ReturnStatus.APPROVED, "Đã duyệt"],
    [ReturnStatus.REJECTED, "Đã từ chối"],
    [ReturnStatus.REFUNDED, "Đã hoàn tiền"],
  ])("lists a return in status %s with no refund button", (status, text) => {
    render(
      <ReturnRequestSection
        orderId="o1"
        orderTotal={50000}
        initialReturns={[{ ...pendingReturn, status, statusText: text }]}
      />,
    );
    expect(screen.getByTestId("return-status")).toHaveTextContent(text);
    expect(screen.queryByRole("button", { name: /Hoàn tiền/ })).toBeNull();
    expect(screen.queryByTestId("return-refund")).toBeNull();
  });

  it("lists every return of the order", () => {
    render(
      <ReturnRequestSection
        orderId="o1"
        orderTotal={50000}
        initialReturns={[
          pendingReturn,
          { ...pendingReturn, id: "r2", statusText: "Đã duyệt" },
        ]}
      />,
    );
    expect(screen.getAllByTestId("return-status")).toHaveLength(2);
  });

  it("shows a return created elsewhere through the shared provider", () => {
    render(
      <ReturnStateProvider initialReturns={[pendingReturn]}>
        <ReturnRequestSection orderId="o1" orderTotal={50000} />
      </ReturnStateProvider>,
    );
    expect(screen.getByTestId("return-status")).toHaveTextContent("Chờ duyệt");
  });
});
