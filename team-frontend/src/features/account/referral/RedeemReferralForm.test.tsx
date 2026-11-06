import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { trackEcommerce, redeemReferralAction, toast } = vi.hoisted(() => ({
  trackEcommerce: vi.fn(),
  redeemReferralAction: vi.fn(),
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));
vi.mock("@/lib/analytics", () => ({ trackEcommerce }));
vi.mock("./actions", () => ({ redeemReferralAction }));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));

import { RedeemReferralForm } from "./RedeemReferralForm";

beforeEach(() => vi.clearAllMocks());

describe("RedeemReferralForm loop events", () => {
  it("tracks referral_redeem without the code", async () => {
    redeemReferralAction.mockResolvedValue({ ok: true });
    render(<RedeemReferralForm />);
    fireEvent.change(screen.getByPlaceholderText("Nhập mã của bạn bè"), {
      target: { value: "ABC123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Nhập mã" }));
    await waitFor(() => expect(trackEcommerce).toHaveBeenCalledTimes(1));
    expect(trackEcommerce.mock.calls[0]).toEqual(["referral_redeem"]);
    expect(JSON.stringify(trackEcommerce.mock.calls)).not.toContain("ABC123");
  });

  it("tracks nothing on a rejected code: ok false", async () => {
    redeemReferralAction.mockResolvedValue({ ok: false, error: "Sai mã" });
    render(<RedeemReferralForm />);
    fireEvent.change(screen.getByPlaceholderText("Nhập mã của bạn bè"), {
      target: { value: "BAD" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Nhập mã" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Sai mã"));
    expect(trackEcommerce).not.toHaveBeenCalled();
  });

  it("tracks nothing on a rejected code: empty code keeps the button disabled", () => {
    render(<RedeemReferralForm />);
    const btn = screen.getByRole("button", { name: "Nhập mã" });
    expect((btn as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(btn);
    expect(redeemReferralAction).not.toHaveBeenCalled();
    expect(trackEcommerce).not.toHaveBeenCalled();
  });
});

describe("RedeemReferralForm 7-day rule", () => {
  it("S22 renders the rule text under the redeem field", () => {
    render(<RedeemReferralForm />);
    expect(
      screen.getByText(
        "Chỉ nhập được trong 7 ngày đầu và khi bạn chưa có đơn hoàn thành.",
      ),
    ).toBeInTheDocument();
  });

  it("S23 shows the refusal in the error toast and under the field", async () => {
    const msg =
      "Không thể nhập mã: tài khoản đã quá 7 ngày, đã có đơn hoàn thành, đã nhập mã hoặc mã là của chính bạn.";
    redeemReferralAction.mockResolvedValue({ ok: false, error: msg });
    render(<RedeemReferralForm />);
    const input = screen.getByPlaceholderText("Nhập mã của bạn bè");
    fireEvent.change(input, { target: { value: "ABC123" } });
    fireEvent.click(screen.getByRole("button", { name: "Nhập mã" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(msg));
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAccessibleDescription(msg);
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("S24 an accepted code toasts Đã ghi nhận mã giới thiệu., clears the field and tracks once", async () => {
    redeemReferralAction.mockResolvedValue({ ok: true });
    render(<RedeemReferralForm />);
    const input = screen.getByPlaceholderText("Nhập mã của bạn bè");
    fireEvent.change(input, { target: { value: "ABC123" } });
    fireEvent.click(screen.getByRole("button", { name: "Nhập mã" }));
    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith("Đã ghi nhận mã giới thiệu."),
    );
    expect(input).toHaveValue("");
    expect(trackEcommerce).toHaveBeenCalledTimes(1);
    expect(trackEcommerce).toHaveBeenCalledWith("referral_redeem");
  });

  it("S25 a guest refusal toasts the gateway message and keeps the field value", async () => {
    redeemReferralAction.mockResolvedValue({
      ok: false,
      error: "Vui lòng đăng nhập.",
    });
    render(<RedeemReferralForm />);
    const input = screen.getByPlaceholderText("Nhập mã của bạn bè");
    fireEvent.change(input, { target: { value: "ABC123" } });
    fireEvent.click(screen.getByRole("button", { name: "Nhập mã" }));
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Vui lòng đăng nhập."),
    );
    expect(input).toHaveValue("ABC123");
    expect(trackEcommerce).not.toHaveBeenCalled();
  });
});
