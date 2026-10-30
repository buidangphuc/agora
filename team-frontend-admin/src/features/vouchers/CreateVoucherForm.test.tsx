import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CreateVoucherForm } from "./CreateVoucherForm";
import { createPlatformVoucherAction } from "./actions";
import { VOUCHER_MESSAGES } from "./voucherMessages";

vi.mock("./actions", () => ({ createPlatformVoucherAction: vi.fn() }));

const stored = {
  id: "v-77",
  code: "SALE11",
  scope: "platform" as const,
  sellerId: "",
  discountType: "percent" as const,
  discountValue: 10,
  minSpend: 0,
  maxDiscount: 0,
  quota: 100,
  used: 0,
  startsAt: "2098-12-31T17:00:00.000Z",
  endsAt: "2099-01-31T17:00:00.000Z",
};

beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

function fill() {
  const set = (label: RegExp, value: string) =>
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
  set(/^Mã voucher/, "sale11");
  set(/^Phần trăm giảm/, "10");
  set(/^Lượt dùng tối đa/, "100");
  set(/^Bắt đầu/, "2099-01-01T00:00");
  set(/^Kết thúc/, "2099-02-01T00:00");
}

const review = () =>
  fireEvent.click(
    screen.getByRole("button", { name: "Xem lại trước khi tạo" }),
  );
const dialog = () => screen.getByRole("dialog");
const createButton = () =>
  within(dialog()).getByRole("button", { name: "Tạo voucher" });

describe("CreateVoucherForm", () => {
  it("shows field errors and opens no dialog for an empty form", () => {
    render(<CreateVoucherForm />);
    review();
    expect(screen.getByText(/Mã gồm 3 đến 32 ký tự/)).toBeInTheDocument();
    expect(screen.getByText("Chọn thời điểm bắt đầu.")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(createPlatformVoucherAction).not.toHaveBeenCalled();
  });

  it("hides the cap field for a fixed discount", () => {
    render(<CreateVoucherForm />);
    expect(screen.getByLabelText(/^Giảm tối đa/)).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Loại giảm giá"), {
      target: { value: "fixed" },
    });
    expect(screen.queryByLabelText(/^Giảm tối đa/)).toBeNull();
    expect(screen.getByLabelText(/^Số tiền giảm/)).toBeInTheDocument();
  });

  it("asks for a second confirmation before creating", async () => {
    vi.mocked(createPlatformVoucherAction).mockResolvedValue({
      ok: true,
      voucher: stored,
    });
    render(<CreateVoucherForm />);
    fill();
    review();
    const summary = within(dialog()).getByTestId("voucher-confirm-summary");
    expect(summary).toHaveTextContent("SALE11");
    expect(summary).toHaveTextContent("Toàn sàn");
    expect(createButton()).toBeDisabled();
    fireEvent.click(createButton());
    expect(createPlatformVoucherAction).not.toHaveBeenCalled();

    fireEvent.click(within(dialog()).getByTestId("voucher-confirm-ack"));
    expect(createButton()).toBeEnabled();
    fireEvent.click(createButton());

    await waitFor(() =>
      expect(screen.getByTestId("voucher-created")).toBeInTheDocument(),
    );
    expect(createPlatformVoucherAction).toHaveBeenCalledTimes(1);
    expect(
      vi.mocked(createPlatformVoucherAction).mock.calls[0][0],
    ).toMatchObject({ code: "SALE11", discountValue: "10", quota: "100" });
    const details = screen.getByTestId("voucher-details");
    expect(details).toHaveTextContent("v-77");
    expect(details).toHaveTextContent("0 / 100");
    expect(screen.getByRole("link", { name: "Xem chi tiết" })).toHaveAttribute(
      "href",
      "/admin/vouchers/SALE11",
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("keeps the dialog open with the audit-down message", async () => {
    vi.mocked(createPlatformVoucherAction).mockResolvedValue({
      ok: false,
      error: VOUCHER_MESSAGES.auditDown,
    });
    render(<CreateVoucherForm />);
    fill();
    review();
    fireEvent.click(within(dialog()).getByTestId("voucher-confirm-ack"));
    fireEvent.click(createButton());
    await waitFor(() =>
      expect(screen.getByTestId("voucher-create-error")).toHaveTextContent(
        "hệ thống kiểm toán",
      ),
    );
    expect(dialog()).toBeInTheDocument();
    expect(screen.queryByTestId("voucher-created")).toBeNull();
  });

  it("goes back to the form with server field errors", async () => {
    vi.mocked(createPlatformVoucherAction).mockResolvedValue({
      ok: false,
      error: VOUCHER_MESSAGES.fields,
      fieldErrors: { endsAt: "Thời điểm kết thúc phải ở tương lai." },
    });
    render(<CreateVoucherForm />);
    fill();
    review();
    fireEvent.click(within(dialog()).getByTestId("voucher-confirm-ack"));
    fireEvent.click(createButton());
    await waitFor(() =>
      expect(
        screen.getByText("Thời điểm kết thúc phải ở tương lai."),
      ).toBeInTheDocument(),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
