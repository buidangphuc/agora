import { render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";

import { ConfirmReceiptButton } from "./ConfirmReceiptButton";
import { confirmReceiptAction } from "./actions";

vi.mock("./actions", () => ({ confirmReceiptAction: vi.fn() }));
const refresh = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), refresh }),
}));
const toast = { success: vi.fn(), error: vi.fn(), info: vi.fn() };
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));

beforeEach(() => vi.clearAllMocks());

describe("ConfirmReceiptButton", () => {
  it("Order lifecycle: confirming receipt calls the server and completes the order - confirm calls ConfirmReceipt and revalidates", async () => {
    vi.mocked(confirmReceiptAction).mockResolvedValue({
      ok: true,
      data: undefined,
    } as never);
    render(<ConfirmReceiptButton orderId="o1" />);
    const user = setupUser();
    await user.click(screen.getByRole("button", { name: "Đã nhận được hàng" }));
    expect(
      screen.getByText("Xác nhận bạn đã nhận được hàng?", { exact: false }),
    ).toBeInTheDocument();
    expect(confirmReceiptAction).not.toHaveBeenCalled();

    await user.click(screen.getByTestId("receipt-confirm"));
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    expect(confirmReceiptAction).toHaveBeenCalledTimes(1);
    expect(confirmReceiptAction).toHaveBeenCalledWith("o1");
    expect(toast.success).toHaveBeenCalledWith("Đã xác nhận nhận hàng.");
    expect(toast.error).not.toHaveBeenCalled();
    await waitFor(() =>
      expect(screen.queryByTestId("receipt-confirm")).toBeNull(),
    );
  });

  it("Order lifecycle: leaving the dialog sends nothing - cancelling the dialog sends nothing", async () => {
    render(<ConfirmReceiptButton orderId="o1" />);
    const user = setupUser();
    await user.click(screen.getByRole("button", { name: "Đã nhận được hàng" }));
    await user.click(screen.getByRole("button", { name: "Chưa" }));
    expect(screen.queryByTestId("receipt-confirm")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Đã nhận được hàng" }));
    await user.keyboard("{Escape}");
    expect(screen.queryByTestId("receipt-confirm")).toBeNull();

    expect(confirmReceiptAction).not.toHaveBeenCalled();
    expect(toast.success).not.toHaveBeenCalled();
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("Order lifecycle: a refused confirm shows the server error - toasts the action error", async () => {
    vi.mocked(confirmReceiptAction).mockResolvedValue({
      ok: false,
      error: "Đơn hàng đã được hoàn thành.",
    });
    render(<ConfirmReceiptButton orderId="o1" />);
    const user = setupUser();
    await user.click(screen.getByRole("button", { name: "Đã nhận được hàng" }));
    await user.click(screen.getByTestId("receipt-confirm"));
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Đơn hàng đã được hoàn thành."),
    );
    expect(toast.success).not.toHaveBeenCalled();
  });
});
