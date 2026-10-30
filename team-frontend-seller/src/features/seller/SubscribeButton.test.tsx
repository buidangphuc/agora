import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { SubscribeButton } from "./SubscribeButton";
import { subscribeAction } from "./actions";

vi.mock("./actions", () => ({ subscribeAction: vi.fn() }));
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
}));

beforeEach(() => vi.clearAllMocks());

describe("SubscribeButton", () => {
  it("the current plan is a disabled Gói hiện tại button", () => {
    render(<SubscribeButton planId="p1" current />);
    expect(screen.getByRole("button", { name: "Gói hiện tại" })).toBeDisabled();
  });

  it("subscribes and toasts", async () => {
    vi.mocked(subscribeAction).mockResolvedValue({ ok: true });
    render(<SubscribeButton planId="p2" current={false} />);
    fireEvent.click(screen.getByRole("button", { name: "Đăng ký" }));
    await waitFor(() =>
      expect(toastSuccess).toHaveBeenCalledWith("Đã đăng ký gói"),
    );
    expect(subscribeAction).toHaveBeenCalledWith("p2");
  });

  it("toasts the error on failure", async () => {
    vi.mocked(subscribeAction).mockResolvedValue({
      ok: false,
      error: "Hết hạn",
    });
    render(<SubscribeButton planId="p2" current={false} />);
    fireEvent.click(screen.getByRole("button", { name: "Đăng ký" }));
    await waitFor(() => expect(toastError).toHaveBeenCalledWith("Hết hạn"));
  });
});
