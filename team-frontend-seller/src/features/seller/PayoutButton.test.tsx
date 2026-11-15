import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PayoutButton, validatePayoutAmount } from "./PayoutButton";
import { requestWalletPayoutAction } from "./actions";

vi.mock("./actions", () => ({ requestWalletPayoutAction: vi.fn() }));
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
}));

beforeEach(() => vi.clearAllMocks());

describe("PayoutButton", () => {
  it("is disabled with an explanation when the balance is 0", () => {
    render(<PayoutButton sellerId="s1" balance={0} />);
    const button = screen.getByRole("button", { name: "Rút tiền" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-disabled", "true");
    expect(
      screen.getByText("Số dư bằng 0, chưa thể rút tiền."),
    ).toBeInTheDocument();
  });

  it("asks for confirmation first, then pends and toasts", async () => {
    let resolve: (v: unknown) => void = () => {};
    vi.mocked(requestWalletPayoutAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }) as never,
    );
    render(<PayoutButton sellerId="s1" balance={500000} />);
    fireEvent.click(screen.getByRole("button", { name: "Rút tiền" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(requestWalletPayoutAction).not.toHaveBeenCalled();
    expect(screen.getByLabelText(/Số tiền/)).toHaveValue(500000);

    fireEvent.click(screen.getByRole("button", { name: "Xác nhận rút tiền" }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Xác nhận rút tiền" }),
      ).toBeDisabled(),
    );
    expect(requestWalletPayoutAction).toHaveBeenCalledWith("s1", 500000);
    resolve({ ok: true });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(toastSuccess).toHaveBeenCalledWith("Đã tạo lệnh rút tiền");
  });

  it("blocks an amount above the balance", () => {
    render(<PayoutButton sellerId="s1" balance={1000} />);
    fireEvent.click(screen.getByRole("button", { name: "Rút tiền" }));
    fireEvent.change(screen.getByLabelText(/Số tiền/), {
      target: { value: "2000" },
    });
    expect(
      screen.getByText("Số tiền vượt quá số dư khả dụng."),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Xác nhận rút tiền" }),
    ).toBeDisabled();
  });

  it("keeps the Modal open with an Alert on failure", async () => {
    vi.mocked(requestWalletPayoutAction).mockResolvedValue({
      ok: false,
      error: "Từ chối",
    });
    render(<PayoutButton sellerId="s1" balance={1000} />);
    fireEvent.click(screen.getByRole("button", { name: "Rút tiền" }));
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận rút tiền" }));
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("Từ chối"),
    );
    expect(toastError).toHaveBeenCalledWith("Từ chối");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});

describe("seller-kyc-ui: payout verification messages", () => {
  const VERIFY = "Cần xác minh danh tính để rút tiền";
  const RETRY = "Chưa kiểm tra được trạng thái xác minh, vui lòng thử lại sau";

  it("A refused payout shows the verification message with a link", async () => {
    vi.mocked(requestWalletPayoutAction).mockResolvedValue({
      ok: false,
      error: VERIFY,
    });
    render(<PayoutButton sellerId="s1" balance={1000} />);
    fireEvent.click(screen.getByRole("button", { name: "Rút tiền" }));
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận rút tiền" }));
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect(dialog).toHaveTextContent(VERIFY));
    expect(
      within(dialog).getByRole("link", { name: /xác minh/i }),
    ).toHaveAttribute("href", "/seller/verification");
  });

  it("Verification outage during payout shows a retry message and re-enables confirm", async () => {
    vi.mocked(requestWalletPayoutAction).mockResolvedValue({
      ok: false,
      error: RETRY,
    });
    render(<PayoutButton sellerId="s1" balance={1000} />);
    fireEvent.click(screen.getByRole("button", { name: "Rút tiền" }));
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận rút tiền" }));
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(RETRY),
    );
    expect(
      screen.getByRole("button", { name: "Xác nhận rút tiền" }),
    ).toBeEnabled();
    expect(screen.queryByRole("link", { name: /xác minh/i })).toBeNull();
  });

  it("A verified seller sees no notice and can pay out", async () => {
    vi.mocked(requestWalletPayoutAction).mockResolvedValue({ ok: true });
    render(<PayoutButton sellerId="s1" balance={1000} />);
    fireEvent.click(screen.getByRole("button", { name: "Rút tiền" }));
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận rút tiền" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(toastSuccess).toHaveBeenCalledWith("Đã tạo lệnh rút tiền");
    expect(screen.queryByText(VERIFY)).toBeNull();
  });
});

describe("validatePayoutAmount", () => {
  it("accepts a whole amount within the balance", () => {
    expect(validatePayoutAmount("500", 1000)).toBeNull();
    expect(validatePayoutAmount("", 1000)).not.toBeNull();
    expect(validatePayoutAmount("0", 1000)).not.toBeNull();
    expect(validatePayoutAmount("1.5", 1000)).not.toBeNull();
    expect(validatePayoutAmount("1001", 1000)).not.toBeNull();
  });
});
