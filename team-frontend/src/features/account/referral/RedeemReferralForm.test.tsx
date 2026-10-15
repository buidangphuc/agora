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
