import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { redirect } from "next/navigation";
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
vi.mock("next/navigation", () => ({
  redirect: vi.fn((url: string) => {
    throw new Error(`NEXT_REDIRECT:${url}`);
  }),
  notFound: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/referral", () => ({
  getMyReferral: vi.fn(),
  listReferralRewards: vi.fn(),
}));
vi.mock("@/features/account/referral/actions", () => ({
  ensureReferralCodeAction: vi.fn(),
  redeemReferralAction: vi.fn(),
  createReferralInviteLinkAction: vi.fn(),
}));

import {
  createReferralInviteLinkAction,
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
    pendingXu: 0,
    releasedXu: 0,
  });
  vi.mocked(listReferralRewards).mockResolvedValue([]);
});

describe("/account/referral", () => {
  it("S19 shows Empty when there are no rewards", async () => {
    render(await ReferralPage());
    expect(screen.getByText("Chưa có phần thưởng nào.")).toBeInTheDocument();
  });

  it("S17 shows statistics, the code with a copy button and a rewards timeline", async () => {
    vi.mocked(getMyReferral).mockResolvedValue({
      code: "ABC123",
      invitedCount: 3,
      rewardsTotal: 150000,
      pendingXu: 0,
      releasedXu: 50000,
    });
    vi.mocked(listReferralRewards).mockResolvedValue([
      {
        id: "r1",
        amount: 50000,
        reason: "Bạn bè đăng ký",
        createdAt: "01/10",
        status: "RELEASED",
        releaseAt: "",
      },
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

  it("S2 generating a code is pending, then toasts", async () => {
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

  it("S23 an invalid redeem toasts and puts the server message on the field", async () => {
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

  it("S24 a valid redeem clears the field and toasts success", async () => {
    const user = setupUser();
    vi.mocked(redeemReferralAction).mockResolvedValue({ ok: true });
    render(await ReferralPage());
    const input = screen.getByLabelText("Mã của bạn bè");
    await user.type(input, "GOOD");
    await user.click(screen.getByRole("button", { name: "Nhập mã" }));
    expect(toast.success).toHaveBeenCalledWith("Đã ghi nhận mã giới thiệu.");
    expect(input).toHaveValue("");
  });
});

const baseReferral = {
  code: "ABC123",
  invitedCount: 2,
  rewardsTotal: 0,
  pendingXu: 0,
  releasedXu: 0,
};

describe("/account/referral invite link", () => {
  function stubClipboard() {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });
    return writeText;
  }

  it("S1 copies <origin>/s/<shortCode> and toasts Đã sao chép link mời.", async () => {
    const writeText = stubClipboard();
    vi.mocked(getMyReferral).mockResolvedValue(baseReferral);
    vi.mocked(createReferralInviteLinkAction).mockResolvedValue({
      ok: true,
      data: { shortCode: "sh0rt" },
    });
    render(await ReferralPage());
    fireEvent.click(screen.getByRole("button", { name: "Chia sẻ link mời" }));
    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith("Đã sao chép link mời."),
    );
    expect(createReferralInviteLinkAction).toHaveBeenCalledTimes(1);
    expect(writeText).toHaveBeenCalledTimes(1);
    expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/s/sh0rt`);
  });

  it("S2 renders the invite button disabled and keeps generate-code when there is no code", async () => {
    render(await ReferralPage());
    const share = screen.getByRole("button", { name: "Chia sẻ link mời" });
    expect(share).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Tạo mã giới thiệu" }),
    ).toBeVisible();
    fireEvent.click(share);
    expect(createReferralInviteLinkAction).not.toHaveBeenCalled();
  });

  it("S3 shows the error toast and leaves the clipboard unchanged on failure", async () => {
    const writeText = stubClipboard();
    vi.mocked(getMyReferral).mockResolvedValue(baseReferral);
    vi.mocked(createReferralInviteLinkAction).mockResolvedValue({
      ok: false,
      error: "Không tạo được link mời. Vui lòng thử lại.",
    });
    render(await ReferralPage());
    fireEvent.click(screen.getByRole("button", { name: "Chia sẻ link mời" }));
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "Không tạo được link mời. Vui lòng thử lại.",
      ),
    );
    expect(writeText).not.toHaveBeenCalled();
    expect(toast.success).not.toHaveBeenCalledWith("Đã sao chép link mời.");
  });

  it("S3 shows the same error toast when the action throws", async () => {
    const writeText = stubClipboard();
    vi.mocked(getMyReferral).mockResolvedValue(baseReferral);
    vi.mocked(createReferralInviteLinkAction).mockRejectedValue(
      new Error("boom"),
    );
    render(await ReferralPage());
    fireEvent.click(screen.getByRole("button", { name: "Chia sẻ link mời" }));
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "Không tạo được link mời. Vui lòng thử lại.",
      ),
    );
    expect(writeText).not.toHaveBeenCalled();
  });

  it("S4 redirects a guest to login and renders no code, link or totals", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(ReferralPage()).rejects.toThrow("NEXT_REDIRECT");
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Faccount%2Freferral",
    );
    expect(getMyReferral).not.toHaveBeenCalled();
    expect(listReferralRewards).not.toHaveBeenCalled();
  });
});

describe("/account/referral xu totals and statuses", () => {
  it("S17 renders pending and released totals in xu with no currency amount", async () => {
    vi.mocked(getMyReferral).mockResolvedValue({
      ...baseReferral,
      pendingXu: 10000,
      releasedXu: 20000,
    });
    const { container } = render(await ReferralPage());
    expect(screen.getByText("Xu đang chờ")).toBeInTheDocument();
    expect(screen.getByText("Xu đã nhận")).toBeInTheDocument();
    expect(screen.getByText("10.000 xu")).toBeInTheDocument();
    expect(screen.getByText("20.000 xu")).toBeInTheDocument();
    expect(container.textContent ?? "").not.toMatch(/\d\s*[đ₫]/i);
  });

  it("S18 shows a status badge per reward with release date, and strikes through VOID", async () => {
    vi.mocked(getMyReferral).mockResolvedValue({
      ...baseReferral,
      pendingXu: 10000,
      releasedXu: 20000,
    });
    vi.mocked(listReferralRewards).mockResolvedValue([
      {
        id: "p",
        amount: 10000,
        reason: "Thưởng chờ",
        createdAt: "01/11",
        status: "PENDING",
        releaseAt: "20/11/2026",
      },
      {
        id: "r",
        amount: 20000,
        reason: "Thưởng nhận",
        createdAt: "02/11",
        status: "RELEASED",
        releaseAt: "",
      },
      {
        id: "v",
        amount: 30000,
        reason: "Thưởng hủy",
        createdAt: "03/11",
        status: "VOID",
        releaseAt: "",
      },
    ]);
    render(await ReferralPage());

    const pending = screen.getByText("Thưởng chờ").closest("li") as HTMLElement;
    expect(within(pending).getByText("Đang chờ")).toBeInTheDocument();
    expect(
      within(pending).getByText(/Dự kiến nhận: 20\/11\/2026/),
    ).toBeInTheDocument();

    const released = screen
      .getByText("Thưởng nhận")
      .closest("li") as HTMLElement;
    expect(within(released).getByText("Đã nhận")).toBeInTheDocument();
    expect(within(released).queryByText(/Dự kiến nhận/)).toBeNull();

    const voided = screen.getByText("Thưởng hủy").closest("li") as HTMLElement;
    expect(within(voided).getByText("Đã hủy")).toBeInTheDocument();
    const amount = within(voided).getByText(/30\.000 xu/);
    const struck =
      amount.closest("del, s, .line-through") ??
      (amount.className.includes("line-through") ? amount : null);
    expect(struck).not.toBeNull();
    expect(within(pending).getByText(/10\.000 xu/)).toBeInTheDocument();
  });

  it("S19 renders 0 xu twice and the empty text when nothing is earned", async () => {
    render(await ReferralPage());
    expect(screen.getAllByText("0 xu")).toHaveLength(2);
    expect(screen.getByText("Chưa có phần thưởng nào.")).toBeInTheDocument();
  });

  it("S20 still renders code and totals with an empty timeline when rewards fail to load", async () => {
    vi.mocked(getMyReferral).mockResolvedValue({
      ...baseReferral,
      pendingXu: 10000,
      releasedXu: 20000,
    });
    vi.mocked(listReferralRewards).mockResolvedValue([]);
    render(await ReferralPage());
    expect(screen.getByTestId("referral-code")).toHaveTextContent("ABC123");
    expect(screen.getByText("10.000 xu")).toBeInTheDocument();
    expect(screen.getByText("20.000 xu")).toBeInTheDocument();
    expect(screen.getByText("Chưa có phần thưởng nào.")).toBeInTheDocument();
  });

  it("S22 shows the 7-day rule under the redeem field", async () => {
    render(await ReferralPage());
    expect(
      screen.getByText(
        "Chỉ nhập được trong 7 ngày đầu và khi bạn chưa có đơn hoàn thành.",
      ),
    ).toBeInTheDocument();
  });
});
