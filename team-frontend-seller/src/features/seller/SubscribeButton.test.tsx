import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { SubscribeButton } from "./SubscribeButton";
import { subscribeAction } from "./actions";

vi.mock("./actions", () => ({ subscribeAction: vi.fn() }));
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
}));

const PERIOD_END = "2026-12-13T10:00:00+07:00";

const upgrade = () => (
  <SubscribeButton
    planId="pro"
    price={199000}
    termDays={30}
    direction="upgrade"
  />
);

const openUpgrade = () => {
  render(upgrade());
  fireEvent.click(screen.getByRole("button", { name: "Nâng cấp" }));
};

const confirm = () =>
  screen.getByRole("button", { name: "Xác nhận đăng ký" });

beforeEach(() => vi.clearAllMocks());

describe("SubscribeButton", () => {
  it("current plan cannot be re-subscribed: disabled Gói hiện tại button", () => {
    render(
      <SubscribeButton
        planId="free"
        price={0}
        termDays={30}
        direction="current"
      />,
    );
    const button = screen.getByRole("button", { name: "Gói hiện tại" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-disabled", "true");
  });

  it("downgrade button reads Hạ gói and is enabled", () => {
    render(
      <SubscribeButton
        planId="free"
        price={0}
        termDays={30}
        direction="downgrade"
        periodEnd={PERIOD_END}
      />,
    );
    expect(screen.getByRole("button", { name: "Hạ gói" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: "Đăng ký" })).toBeNull();
  });

  it("upgrade dialog states price before any write", () => {
    openUpgrade();
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent("₫199.000");
    expect(dialog).toHaveTextContent("số dư có thể rút");
    expect(dialog).toHaveTextContent(
      "Nâng cấp có hiệu lực ngay, tính đủ giá gói mới, không hoàn tiền phần còn lại, chu kỳ 30 ngày mới.",
    );
    expect(subscribeAction).not.toHaveBeenCalled();
  });

  it("downgrade dialog names the period end", () => {
    render(
      <SubscribeButton
        planId="free"
        price={0}
        termDays={30}
        direction="downgrade"
        periodEnd={PERIOD_END}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Hạ gói" }));
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent(
      "Hạ gói có hiệu lực từ ngày 13/12/2026 (hết chu kỳ hiện tại).",
    );
    expect(dialog).not.toHaveTextContent("Nâng cấp có hiệu lực ngay");
    expect(subscribeAction).not.toHaveBeenCalled();
  });

  it("confirm calls subscribe once", async () => {
    let resolve: (v: unknown) => void = () => {};
    vi.mocked(subscribeAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }) as never,
    );
    openUpgrade();
    fireEvent.click(confirm());
    await waitFor(() => expect(confirm()).toBeDisabled());
    fireEvent.click(confirm());
    expect(subscribeAction).toHaveBeenCalledTimes(1);
    expect(subscribeAction).toHaveBeenCalledWith("pro");
    resolve({ ok: true });
    await waitFor(() =>
      expect(toastSuccess).toHaveBeenCalledWith("Đã đăng ký gói"),
    );
    expect(toastError).not.toHaveBeenCalled();
  });

  it("denied subscribe shows an error and no change", async () => {
    vi.mocked(subscribeAction).mockResolvedValue({
      ok: false,
      error: "Đăng ký gói thất bại.",
    });
    openUpgrade();
    fireEvent.click(confirm());
    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith("Đăng ký gói thất bại."),
    );
    expect(toastSuccess).not.toHaveBeenCalled();
    expect(screen.queryByRole("link", { name: "Xem ví" })).toBeNull();
  });

  it("insufficient balance is explained with a wallet link and nothing changes", async () => {
    const message = "Số dư có thể rút không đủ để trả gói này.";
    vi.mocked(subscribeAction).mockResolvedValue({ ok: false, error: message });
    openUpgrade();
    fireEvent.click(confirm());
    await waitFor(() => expect(toastError).toHaveBeenCalledWith(message));
    expect(
      within(screen.getByRole("dialog")).getByRole("alert"),
    ).toHaveTextContent(message);
    expect(screen.getByRole("link", { name: "Xem ví" })).toHaveAttribute(
      "href",
      "/seller/wallet",
    );
    expect(toastSuccess).not.toHaveBeenCalled();
  });
});
