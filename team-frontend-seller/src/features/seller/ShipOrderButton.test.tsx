import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { updateOrderStatusAction } from "@/features/order/actions";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { ShipOrderButton } from "./ShipOrderButton";

vi.mock("@/features/order/actions", () => ({
  updateOrderStatusAction: vi.fn(),
}));
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
}));

beforeEach(() => vi.clearAllMocks());

describe("ShipOrderButton (row action)", () => {
  it("ships directly, pending then success toast, without inventing a tracking number", async () => {
    let resolve: (v: unknown) => void = () => {};
    vi.mocked(updateOrderStatusAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }) as never,
    );
    render(<ShipOrderButton orderId="o1" />);
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận gửi" }));

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Xác nhận gửi" }),
      ).toBeDisabled(),
    );
    expect(updateOrderStatusAction).toHaveBeenCalledWith(
      "o1",
      OrderStatus.SHIPPED,
      undefined,
    );
    resolve({ ok: true });
    await waitFor(() =>
      expect(toastSuccess).toHaveBeenCalledWith("Đã bàn giao vận chuyển"),
    );
    expect(screen.getByRole("button", { name: "Xác nhận gửi" })).toBeEnabled();
  });

  it("reports a failure with an error toast", async () => {
    vi.mocked(updateOrderStatusAction).mockResolvedValue({
      ok: false,
      error: "Trạng thái không hợp lệ",
    });
    render(<ShipOrderButton orderId="o1" />);
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận gửi" }));
    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith("Trạng thái không hợp lệ"),
    );
  });
});

describe("ShipOrderButton (confirm Modal)", () => {
  function open() {
    render(
      <ShipOrderButton
        orderId="o1"
        label="Bàn giao vận chuyển"
        variant="primary"
        withConfirm
      />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Bàn giao vận chuyển" }),
    );
  }

  it("asks first and sends nothing until confirmed", () => {
    open();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(updateOrderStatusAction).not.toHaveBeenCalled();
  });

  it("confirms with the optional tracking number, then closes with a toast", async () => {
    vi.mocked(updateOrderStatusAction).mockResolvedValue({ ok: true });
    open();
    fireEvent.change(screen.getByLabelText("Mã vận đơn (nếu có)"), {
      target: { value: " SPX123 " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận bàn giao" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(updateOrderStatusAction).toHaveBeenCalledWith(
      "o1",
      OrderStatus.SHIPPED,
      "SPX123",
    );
    expect(toastSuccess).toHaveBeenCalledWith("Đã bàn giao vận chuyển");
  });

  it("a failure keeps the Modal open with an Alert and toast", async () => {
    vi.mocked(updateOrderStatusAction).mockResolvedValue({
      ok: false,
      error: "Đơn đã được giao",
    });
    open();
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận bàn giao" }));
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("Đơn đã được giao"),
    );
    expect(toastError).toHaveBeenCalledWith("Đơn đã được giao");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Xác nhận bàn giao" }),
    ).toBeEnabled();
  });
});
