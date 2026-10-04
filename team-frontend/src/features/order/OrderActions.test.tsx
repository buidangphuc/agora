import { render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";

import { OrderActions, ReorderButton } from "./OrderActions";
import { ReturnStateProvider } from "./ReturnState";
import {
  cancelOrderAction,
  createReturnRequestAction,
  reorderAction,
} from "./actions";

vi.mock("./actions", () => ({
  cancelOrderAction: vi.fn(),
  reorderAction: vi.fn(),
  createReturnRequestAction: vi.fn(),
  mockRefundAction: vi.fn(),
}));

const push = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push }),
}));

const toast = { success: vi.fn(), error: vi.fn(), info: vi.fn() };
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => toast,
}));

function deferred<T>() {
  let resolve: (v: T) => void = () => {};
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

function renderActions(
  props: Partial<React.ComponentProps<typeof OrderActions>> = {},
) {
  return render(
    <ReturnStateProvider>
      <OrderActions
        orderId="o1"
        orderTotal={50000}
        canCancel
        canReturn={false}
        {...props}
      />
    </ReturnStateProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe("OrderActions reorder", () => {
  it("shows pending, then toasts and goes to the cart", async () => {
    const d = deferred<Awaited<ReturnType<typeof reorderAction>>>();
    vi.mocked(reorderAction).mockReturnValue(d.promise);
    renderActions();
    const user = setupUser();
    await user.click(screen.getByRole("button", { name: "Mua lại" }));

    const button = screen.getByRole("button", { name: "Mua lại" });
    await waitFor(() => expect(button).toBeDisabled());
    expect(button).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("button", { name: "Hủy đơn" })).toBeDisabled();

    d.resolve({ ok: true, data: { totalItems: 2 } });
    await waitFor(() => expect(push).toHaveBeenCalledWith("/cart"));
    expect(reorderAction).toHaveBeenCalledWith("o1");
    expect(toast.success).toHaveBeenCalled();
    await waitFor(() => expect(button).toBeEnabled());
  });

  it("toasts the error and stays put on failure", async () => {
    vi.mocked(reorderAction).mockResolvedValue({ ok: false, error: "empty" });
    renderActions();
    const user = setupUser();
    await user.click(screen.getByRole("button", { name: "Mua lại" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("empty"));
    expect(push).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Mua lại" })).toBeEnabled();
  });

  it("ReorderButton calls the same action", async () => {
    vi.mocked(reorderAction).mockResolvedValue({ ok: true });
    render(<ReorderButton orderId="o9" />);
    const user = setupUser();
    await user.click(screen.getByRole("button", { name: "Mua lại" }));
    await waitFor(() => expect(reorderAction).toHaveBeenCalledWith("o9"));
  });
});

describe("OrderActions cancel", () => {
  async function openCancel(user: ReturnType<typeof setupUser>) {
    await user.click(screen.getByRole("button", { name: "Hủy đơn" }));
  }

  it("calls cancelOrderAction once, with a pending state, then closes and toasts", async () => {
    const d = deferred<Awaited<ReturnType<typeof cancelOrderAction>>>();
    vi.mocked(cancelOrderAction).mockReturnValue(d.promise);
    renderActions();
    const user = setupUser();
    await openCancel(user);
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    await user.click(screen.getByTestId("cancel-confirm"));
    const confirm = screen.getByTestId("cancel-confirm");
    await waitFor(() => expect(confirm).toBeDisabled());
    expect(confirm).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("button", { name: "Không hủy" })).toBeDisabled();

    // The Modal cannot be dismissed while pending.
    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    d.resolve({ ok: true });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(cancelOrderAction).toHaveBeenCalledTimes(1);
    expect(cancelOrderAction).toHaveBeenCalledWith(
      "o1",
      "Tôi muốn thay đổi địa chỉ hoặc đổi ý",
    );
    expect(toast.success).toHaveBeenCalled();
  });

  it("keeps the Modal open and enabled when the action fails", async () => {
    vi.mocked(cancelOrderAction).mockResolvedValue({ ok: false, error: "no" });
    renderActions();
    const user = setupUser();
    await openCancel(user);
    await user.click(screen.getByTestId("cancel-confirm"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("no"));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByTestId("cancel-confirm")).toBeEnabled();
  });

  it("does not offer cancel when the order is not cancellable", () => {
    renderActions({ canCancel: false });
    expect(screen.queryByRole("button", { name: "Hủy đơn" })).toBeNull();
  });
});

describe("OrderActions return", () => {
  it("is hidden unless the order is eligible", () => {
    renderActions({ canReturn: false });
    expect(
      screen.queryByRole("button", { name: "Yêu cầu trả hàng" }),
    ).toBeNull();
  });

  it("opens the request Modal, submits, and hides the trigger once a return exists", async () => {
    vi.mocked(createReturnRequestAction).mockResolvedValue({
      ok: true,
      data: {
        id: "r1",
        orderId: "o1",
        reason: "x",
        refundAmount: 50000,
        status: 1,
        statusText: "Chờ duyệt",
      },
    });
    renderActions({ canReturn: true });
    const user = setupUser();
    await user.click(screen.getByRole("button", { name: "Yêu cầu trả hàng" }));
    await user.selectOptions(screen.getByTestId("return-reason"), "defective");
    await user.click(screen.getByTestId("return-submit"));

    await waitFor(() => expect(createReturnRequestAction).toHaveBeenCalled());
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Yêu cầu trả hàng" }),
      ).toBeNull(),
    );
    expect(toast.success).toHaveBeenCalled();
  });
});
