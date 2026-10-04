import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { processMockPaymentAction } from "@/features/order/actions";
import { setupUser } from "@/test/user";

import { PaymentSimulator } from "./PaymentSimulator";

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  showToast: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));
vi.mock("@/features/order/actions", () => ({
  processMockPaymentAction: vi.fn(),
}));

type Res = Awaited<ReturnType<typeof processMockPaymentAction>>;

function deferred() {
  let resolve!: (v: Res) => void;
  const promise = new Promise<Res>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

const props = { orderId: "order-1", transactionId: "tx1" } as const;

beforeEach(() => vi.clearAllMocks());

describe("PaymentSimulator", () => {
  it("disables both buttons while pending and loads only the pressed one", async () => {
    const user = setupUser();
    const d = deferred();
    vi.mocked(processMockPaymentAction).mockReturnValue(d.promise);
    render(<PaymentSimulator {...props} initialPhase="idle" />);

    await user.click(
      screen.getByRole("button", { name: "Thanh toán thành công" }),
    );
    const ok = screen.getByRole("button", { name: "Thanh toán thành công" });
    const bad = screen.getByRole("button", { name: "Thanh toán thất bại" });
    expect(ok).toBeDisabled();
    expect(bad).toBeDisabled();
    expect(ok).toHaveAttribute("aria-busy", "true");
    expect(bad).not.toHaveAttribute("aria-busy");
    await user.click(bad);
    expect(processMockPaymentAction).toHaveBeenCalledTimes(1);
    expect(processMockPaymentAction).toHaveBeenCalledWith("tx1", true);

    await act(async () => {
      d.resolve({ ok: true, data: { message: "paid" } });
      await d.promise;
    });
  });

  it("shows a success Result with links to the orders and home", async () => {
    const user = setupUser();
    vi.mocked(processMockPaymentAction).mockResolvedValue({
      ok: true,
      data: { message: "paid" },
    });
    render(<PaymentSimulator {...props} initialPhase="idle" />);
    await user.click(
      screen.getByRole("button", { name: "Thanh toán thành công" }),
    );
    expect(
      screen.getByText("Thanh toán thành công", { selector: "h2" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Xem đơn hàng" })).toHaveAttribute(
      "href",
      "/account/orders",
    );
    expect(
      screen.getByRole("link", { name: "Tiếp tục mua sắm" }),
    ).toHaveAttribute("href", "/");
    expect(toast.success).toHaveBeenCalled();
  });

  it("shows an error Result with Thử lại and Đổi phương thức and an error toast on failure", async () => {
    const user = setupUser();
    vi.mocked(processMockPaymentAction).mockResolvedValue({
      ok: false,
      error: "Thẻ bị từ chối",
    });
    render(<PaymentSimulator {...props} initialPhase="idle" />);
    await user.click(
      screen.getByRole("button", { name: "Thanh toán thất bại" }),
    );
    expect(
      screen.getByText("Thanh toán thất bại", { selector: "h2" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Thẻ bị từ chối")).toBeInTheDocument();
    expect(toast.error).toHaveBeenCalledWith("Thẻ bị từ chối");
    expect(
      screen.getByRole("link", { name: "Đổi phương thức" }),
    ).toHaveAttribute("href", "/account/orders/order-1");

    await user.click(screen.getByRole("button", { name: "Thử lại" }));
    expect(
      screen.getByRole("button", { name: "Thanh toán thành công" }),
    ).toBeEnabled();
  });

  it("renders the settled states straight from the server status", () => {
    const first = render(
      <PaymentSimulator {...props} initialPhase="success" />,
    );
    expect(
      screen.getByRole("link", { name: "Xem đơn hàng" }),
    ).toBeInTheDocument();
    first.unmount();
    render(<PaymentSimulator {...props} initialPhase="failed" />);
    expect(screen.getByRole("button", { name: "Thử lại" })).toBeInTheDocument();
  });

  it("treats a thrown action as a failure and re-enables retry", async () => {
    const user = setupUser();
    vi.mocked(processMockPaymentAction).mockRejectedValue(new Error("net"));
    render(<PaymentSimulator {...props} initialPhase="idle" />);
    await user.click(
      screen.getByRole("button", { name: "Thanh toán thành công" }),
    );
    expect(toast.error).toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Thử lại" })).toBeEnabled();
  });
});
