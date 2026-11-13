import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { CancelSubscriptionButton } from "./CancelSubscriptionButton";
import { cancelSubscriptionAction } from "./actions";

vi.mock("./actions", () => ({ cancelSubscriptionAction: vi.fn() }));
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
}));

const PERIOD_END = "2026-12-13T10:00:00+07:00";

const open = () => {
  render(
    <CancelSubscriptionButton paid autoRenew periodEnd={PERIOD_END} />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Hủy gia hạn" }));
};
const confirm = () =>
  screen.getByRole("button", { name: "Xác nhận hủy gia hạn" });

beforeEach(() => vi.clearAllMocks());

describe("CancelSubscriptionButton", () => {
  it("renders only while auto renew is on", () => {
    const { unmount } = render(
      <CancelSubscriptionButton paid autoRenew periodEnd={PERIOD_END} />,
    );
    expect(screen.getByRole("button", { name: "Hủy gia hạn" })).toBeEnabled();
    unmount();

    const off = render(
      <CancelSubscriptionButton
        paid
        autoRenew={false}
        periodEnd={PERIOD_END}
      />,
    );
    expect(screen.queryByRole("button", { name: "Hủy gia hạn" })).toBeNull();
    expect(
      screen.getByText("Sẽ chuyển về Miễn phí ngày 13/12/2026"),
    ).toBeInTheDocument();
    off.unmount();

    render(<CancelSubscriptionButton paid={false} autoRenew={false} />);
    expect(screen.queryByRole("button", { name: "Hủy gia hạn" })).toBeNull();
    expect(screen.queryByText(/Sẽ chuyển về Miễn phí/)).toBeNull();
  });

  it("dialog states the period end before any write", () => {
    open();
    expect(screen.getByRole("dialog")).toHaveTextContent(
      "Gói vẫn dùng được đến ngày 13/12/2026, sau đó chuyển về Miễn phí.",
    );
    expect(cancelSubscriptionAction).not.toHaveBeenCalled();
  });

  it("confirm cancels once", async () => {
    let resolve: (v: unknown) => void = () => {};
    vi.mocked(cancelSubscriptionAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }) as never,
    );
    open();
    fireEvent.click(confirm());
    await waitFor(() => expect(confirm()).toBeDisabled());
    fireEvent.click(confirm());
    expect(cancelSubscriptionAction).toHaveBeenCalledTimes(1);
    expect(cancelSubscriptionAction).toHaveBeenCalledWith();
    resolve({ ok: true });
    await waitFor(() =>
      expect(toastSuccess).toHaveBeenCalledWith("Đã hủy gia hạn"),
    );
  });

  it("denied cancel shows an error", async () => {
    vi.mocked(cancelSubscriptionAction).mockResolvedValue({
      ok: false,
      error: "Hủy gia hạn thất bại.",
    });
    open();
    fireEvent.click(confirm());
    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith("Hủy gia hạn thất bại."),
    );
    expect(toastSuccess).not.toHaveBeenCalled();
  });

  it("cancel failure keeps renewal on", async () => {
    vi.mocked(cancelSubscriptionAction).mockResolvedValue({
      ok: false,
      error: "Hủy gia hạn thất bại.",
    });
    open();
    fireEvent.click(confirm());
    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(toastSuccess).not.toHaveBeenCalled();
    expect(screen.queryByText(/Sẽ chuyển về Miễn phí ngày/)).toBeNull();
    expect(screen.getByRole("button", { name: "Hủy gia hạn" })).toBeInTheDocument();
  });
});
