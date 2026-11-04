import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { VoucherActions } from "./VoucherActions";
import { deactivateVoucherAction, updateVoucherAction } from "./actions";
import { validateEditForm } from "./voucherForm";
import { VOUCHER_MESSAGES } from "./voucherMessages";

const refresh = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh }),
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("./actions", () => ({
  deactivateVoucherAction: vi.fn(),
  updateVoucherAction: vi.fn(),
}));

const voucher = {
  id: "v-1",
  code: "SALE11",
  scope: "platform" as const,
  sellerId: "",
  discountType: "percent" as const,
  discountValue: 10,
  minSpend: 100000,
  maxDiscount: 50000,
  quota: 100,
  used: 5,
  active: true,
  startsAt: "2098-12-31T17:00:00.000Z",
  endsAt: "2099-01-31T17:00:00.000Z",
};

beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

const stopButton = () => screen.getByRole("button", { name: "Ngừng voucher" });
const editButton = () => screen.getByRole("button", { name: "Sửa voucher" });
const dialog = () => screen.getByRole("dialog");
const confirmStop = () =>
  within(dialog()).getByRole("button", { name: "Xác nhận ngừng" });
const save = () => screen.getByRole("button", { name: "Lưu thay đổi" });
const endsAtInput = () => screen.getByLabelText(/^Kết thúc/);
const quotaInput = () => screen.getByLabelText(/^Lượt dùng tối đa/);

function openEdit() {
  fireEvent.click(editButton());
}

describe("VoucherActions visibility", () => {
  it("V28 shows Stop and Edit on an active voucher", () => {
    render(<VoucherActions voucher={voucher} />);
    expect(stopButton()).toBeInTheDocument();
    expect(editButton()).toBeInTheDocument();
  });

  it("V31 shows neither control on a stopped voucher", () => {
    render(<VoucherActions voucher={{ ...voucher, active: false }} />);
    expect(screen.queryByRole("button", { name: "Ngừng voucher" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Sửa voucher" })).toBeNull();
  });
});

describe("Stop dialog", () => {
  it("V28 stops nothing until the admin confirms in the dialog", () => {
    render(<VoucherActions voucher={voucher} />);
    fireEvent.click(stopButton());
    expect(dialog()).toBeInTheDocument();
    expect(deactivateVoucherAction).not.toHaveBeenCalled();
    fireEvent.click(within(dialog()).getByRole("button", { name: "Quay lại" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(deactivateVoucherAction).not.toHaveBeenCalled();
  });

  it("V28 confirming stops the voucher by id and refreshes the page", async () => {
    vi.mocked(deactivateVoucherAction).mockResolvedValue({
      ok: true,
      voucher: { ...voucher, active: false },
    });
    render(<VoucherActions voucher={voucher} />);
    fireEvent.click(stopButton());
    fireEvent.click(confirmStop());
    await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
    expect(deactivateVoucherAction).toHaveBeenCalledTimes(1);
    expect(deactivateVoucherAction).toHaveBeenCalledWith("v-1");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("V26a shows the audit-down change message in the dialog and V27c not the other", async () => {
    vi.mocked(deactivateVoucherAction).mockResolvedValue({
      ok: false,
      error: VOUCHER_MESSAGES.auditDownChange,
    });
    render(<VoucherActions voucher={voucher} />);
    fireEvent.click(stopButton());
    fireEvent.click(confirmStop());
    await waitFor(() =>
      expect(within(dialog()).getByRole("alert")).toHaveTextContent(
        VOUCHER_MESSAGES.auditDownChange,
      ),
    );
    expect(dialog()).not.toHaveTextContent(
      VOUCHER_MESSAGES.promotionDownChange,
    );
    expect(refresh).not.toHaveBeenCalled();
  });

  it("V27b shows the promotion-down change message in the dialog and V27c not the other", async () => {
    vi.mocked(deactivateVoucherAction).mockResolvedValue({
      ok: false,
      error: VOUCHER_MESSAGES.promotionDownChange,
    });
    render(<VoucherActions voucher={voucher} />);
    fireEvent.click(stopButton());
    fireEvent.click(confirmStop());
    await waitFor(() =>
      expect(within(dialog()).getByRole("alert")).toHaveTextContent(
        VOUCHER_MESSAGES.promotionDownChange,
      ),
    );
    expect(dialog()).not.toHaveTextContent(VOUCHER_MESSAGES.auditDownChange);
    expect(refresh).not.toHaveBeenCalled();
  });
});

describe("Edit form", () => {
  it("V29 prefills the end date in Vietnam time and the current quota", () => {
    render(<VoucherActions voucher={voucher} />);
    openEdit();
    expect(endsAtInput()).toHaveValue("2099-02-01T00:00");
    expect(quotaInput()).toHaveValue("100");
  });

  it("V29a offers no input for discount, min spend, max discount or scope", () => {
    render(<VoucherActions voucher={voucher} />);
    openEdit();
    for (const label of [
      /^Phần trăm giảm/,
      /^Số tiền giảm/,
      /^Giảm tối đa/,
      /^Đơn tối thiểu/,
      /^Phạm vi/,
      /^Loại giảm giá/,
      /^Mã voucher/,
    ]) {
      expect(screen.queryByLabelText(label)).toBeNull();
    }
    expect(screen.getAllByRole("textbox").length).toBeLessThanOrEqual(2);
  });

  it("V29 saves the new end date and quota then refreshes the page", async () => {
    vi.mocked(updateVoucherAction).mockResolvedValue({
      ok: true,
      voucher: {
        ...voucher,
        quota: 200,
        endsAt: "2099-02-28T17:00:00.000Z",
      },
    });
    render(<VoucherActions voucher={voucher} />);
    openEdit();
    fireEvent.change(endsAtInput(), { target: { value: "2099-03-01T00:00" } });
    fireEvent.change(quotaInput(), { target: { value: "200" } });
    fireEvent.click(save());
    await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
    expect(updateVoucherAction).toHaveBeenCalledTimes(1);
    expect(updateVoucherAction).toHaveBeenCalledWith(voucher, {
      endsAt: "2099-03-01T00:00",
      quota: "200",
    });
  });

  it("V30 shows the quota error on the field and sends no request for quota 0", () => {
    render(<VoucherActions voucher={voucher} />);
    openEdit();
    fireEvent.change(quotaInput(), { target: { value: "0" } });
    fireEvent.click(save());
    const expected = validateEditForm(
      { endsAt: "2099-02-01T00:00", quota: "0" },
      voucher,
      new Date(),
    );
    expect(expected.ok).toBe(false);
    if (!expected.ok) {
      expect(
        screen.getByText(expected.errors.quota ?? "never"),
      ).toBeInTheDocument();
    }
    expect(updateVoucherAction).not.toHaveBeenCalled();
  });

  it("V30 shows the quota error for a quota below the current one", () => {
    render(<VoucherActions voucher={voucher} />);
    openEdit();
    fireEvent.change(quotaInput(), { target: { value: "50" } });
    fireEvent.click(save());
    const expected = validateEditForm(
      { endsAt: "2099-02-01T00:00", quota: "50" },
      voucher,
      new Date(),
    );
    if (!expected.ok) {
      expect(
        screen.getByText(expected.errors.quota ?? "never"),
      ).toBeInTheDocument();
    }
    expect(updateVoucherAction).not.toHaveBeenCalled();
  });

  it("V30 shows the end date error for an end not after the start", () => {
    render(<VoucherActions voucher={voucher} />);
    openEdit();
    fireEvent.change(endsAtInput(), { target: { value: "2099-01-01T00:00" } });
    fireEvent.click(save());
    const expected = validateEditForm(
      { endsAt: "2099-01-01T00:00", quota: "100" },
      voucher,
      new Date(),
    );
    expect(expected.ok).toBe(false);
    if (!expected.ok) {
      expect(
        screen.getByText(expected.errors.endsAt ?? "never"),
      ).toBeInTheDocument();
    }
    expect(updateVoucherAction).not.toHaveBeenCalled();
  });

  it("V30a does not render the quota field for an unlimited voucher", () => {
    render(<VoucherActions voucher={{ ...voucher, quota: 0 }} />);
    openEdit();
    expect(endsAtInput()).toBeInTheDocument();
    expect(screen.queryByLabelText(/^Lượt dùng tối đa/)).toBeNull();
  });

  it("V26a shows the audit-down change message and keeps the form open", async () => {
    vi.mocked(updateVoucherAction).mockResolvedValue({
      ok: false,
      error: VOUCHER_MESSAGES.auditDownChange,
    });
    render(<VoucherActions voucher={voucher} />);
    openEdit();
    fireEvent.change(quotaInput(), { target: { value: "200" } });
    fireEvent.click(save());
    await waitFor(() =>
      expect(screen.getByTestId("voucher-edit-error")).toHaveTextContent(
        VOUCHER_MESSAGES.auditDownChange,
      ),
    );
    expect(screen.getByTestId("voucher-edit-error")).not.toHaveTextContent(
      VOUCHER_MESSAGES.promotionDownChange,
    );
    expect(refresh).not.toHaveBeenCalled();
  });

  it("V27b shows the promotion-down change message and keeps the form open", async () => {
    vi.mocked(updateVoucherAction).mockResolvedValue({
      ok: false,
      error: VOUCHER_MESSAGES.promotionDownChange,
    });
    render(<VoucherActions voucher={voucher} />);
    openEdit();
    fireEvent.change(quotaInput(), { target: { value: "200" } });
    fireEvent.click(save());
    await waitFor(() =>
      expect(screen.getByTestId("voucher-edit-error")).toHaveTextContent(
        VOUCHER_MESSAGES.promotionDownChange,
      ),
    );
    expect(screen.getByTestId("voucher-edit-error")).not.toHaveTextContent(
      VOUCHER_MESSAGES.auditDownChange,
    );
    expect(refresh).not.toHaveBeenCalled();
  });
});
