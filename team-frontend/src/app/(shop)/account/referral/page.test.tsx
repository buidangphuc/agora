import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getMyReferral, listReferralRewards } from "@/lib/gateway/referral";
import { getPrincipal } from "@/lib/gateway/session";
import { setupUser } from "@/test/user";

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/referral", () => ({
  getMyReferral: vi.fn(),
  listReferralRewards: vi.fn(),
}));
vi.mock("@/features/account/referral/actions", () => ({
  ensureReferralCodeAction: vi.fn(),
  redeemReferralAction: vi.fn(),
}));

import {
  ensureReferralCodeAction,
  redeemReferralAction,
} from "@/features/account/referral/actions";
import ReferralPage from "./page";

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({ id: "u", name: "t", scopes: [] });
  vi.mocked(getMyReferral).mockResolvedValue({
    code: "",
    invitedCount: 0,
    rewardsTotal: 0,
  });
  vi.mocked(listReferralRewards).mockResolvedValue([]);
});

describe("/account/referral", () => {
  it("shows Empty when there are no rewards", async () => {
    render(await ReferralPage());
    expect(screen.getByText("Chưa có phần thưởng nào.")).toBeInTheDocument();
  });

  it("shows statistics, the code with a copy button and a rewards timeline", async () => {
    vi.mocked(getMyReferral).mockResolvedValue({
      code: "ABC123",
      invitedCount: 3,
      rewardsTotal: 150000,
    });
    vi.mocked(listReferralRewards).mockResolvedValue([
      { id: "r1", amount: 50000, reason: "Bạn bè đăng ký", createdAt: "01/10" },
    ]);
    render(await ReferralPage());
    expect(screen.getByTestId("referral-code")).toHaveTextContent("ABC123");
    expect(screen.getByRole("button", { name: "Sao chép" })).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "Tạo mã giới thiệu" }),
    ).toBeNull();
    expect(screen.getByText("Đã mời")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
    const item = screen.getByText("Bạn bè đăng ký").closest("li");
    expect(item).not.toBeNull();
    expect(within(item as HTMLElement).getByText(/\+/)).toBeInTheDocument();
  });

  it("generating a code is pending, then toasts", async () => {
    const user = setupUser();
    vi.mocked(ensureReferralCodeAction).mockResolvedValue({
      ok: true,
      data: { code: "NEW1" },
    });
    render(await ReferralPage());
    await user.click(screen.getByRole("button", { name: "Tạo mã giới thiệu" }));
    expect(ensureReferralCodeAction).toHaveBeenCalledTimes(1);
    expect(toast.success).toHaveBeenCalledWith("Đã tạo mã giới thiệu của bạn.");
  });

  it("an invalid redeem toasts and puts the server message on the field", async () => {
    const user = setupUser();
    vi.mocked(redeemReferralAction).mockResolvedValue({
      ok: false,
      error: "Mã giới thiệu không hợp lệ.",
    });
    render(await ReferralPage());
    const input = screen.getByLabelText("Mã của bạn bè");
    const submit = screen.getByRole("button", { name: "Nhập mã" });
    expect(submit).toBeDisabled();
    await user.type(input, "BAD");
    await user.click(submit);

    expect(redeemReferralAction).toHaveBeenCalledWith("BAD");
    expect(toast.error).toHaveBeenCalledWith("Mã giới thiệu không hợp lệ.");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAccessibleDescription("Mã giới thiệu không hợp lệ.");
  });

  it("a valid redeem clears the field and toasts success", async () => {
    const user = setupUser();
    vi.mocked(redeemReferralAction).mockResolvedValue({ ok: true });
    render(await ReferralPage());
    const input = screen.getByLabelText("Mã của bạn bè");
    await user.type(input, "GOOD");
    await user.click(screen.getByRole("button", { name: "Nhập mã" }));
    expect(toast.success).toHaveBeenCalledWith("Đã nhập mã giới thiệu.");
    expect(input).toHaveValue("");
  });
});
