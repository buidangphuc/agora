import { render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ViewAddress } from "@/lib/gateway/addresses";
import { setupUser } from "@/test/user";

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));
vi.mock("./actions", () => ({
  createAddressAction: vi.fn(),
  updateAddressAction: vi.fn(),
  deleteAddressAction: vi.fn(),
  setDefaultAddressAction: vi.fn(),
}));

import { AddAddressButton, AddressActions } from "./AddressActions";
import { AddressManager } from "./AddressManager";
import { AddressModal } from "./AddressModal";
import {
  createAddressAction,
  deleteAddressAction,
  setDefaultAddressAction,
} from "./actions";

const home: ViewAddress = {
  id: "a1",
  userId: "u1",
  recipientName: "Nguyễn Văn A",
  phone: "0912345678",
  street: "1 Lê Lợi",
  ward: "P. Bến Nghé",
  district: "Quận 1",
  city: "TP. HCM",
  isDefault: true,
};
const office: ViewAddress = {
  ...home,
  id: "a2",
  recipientName: "Trần Thị B",
  isDefault: false,
};

beforeEach(() => vi.clearAllMocks());

describe("AddressManager", () => {
  it("renders the empty state with its action", () => {
    render(<AddressManager addresses={[]} />);
    expect(
      screen.getByText("Bạn chưa có địa chỉ nhận hàng nào."),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Thêm địa chỉ ngay" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Thêm địa chỉ mới" }),
    ).toBeInTheDocument();
  });

  it("renders cards with a default Tag only on the default address", () => {
    render(<AddressManager addresses={[home, office]} />);
    const cards = screen.getAllByTestId("address-card");
    expect(cards).toHaveLength(2);
    expect(within(cards[0]).getByText("Mặc định")).toBeInTheDocument();
    expect(within(cards[1]).queryByText("Mặc định")).toBeNull();
    expect(
      within(cards[0]).getByText("1 Lê Lợi, P. Bến Nghé, Quận 1, TP. HCM"),
    ).toBeInTheDocument();
    // The default address can only be edited.
    expect(within(cards[0]).queryByRole("button", { name: "Xóa" })).toBeNull();
    expect(within(cards[1]).getByRole("button", { name: "Xóa" })).toBeVisible();
  });
});

function fillRequired(user: ReturnType<typeof setupUser>) {
  return (async () => {
    await user.type(screen.getByLabelText(/Họ và tên/), "Nguyễn Văn A");
    await user.type(screen.getByLabelText(/Số điện thoại/), "0912345678");
    await user.type(screen.getByLabelText(/Địa chỉ chi tiết/), "1 Lê Lợi");
    await user.type(screen.getByLabelText(/Tỉnh \/ Thành phố/), "TP. HCM");
  })();
}

describe("AddressModal", () => {
  it("adds an address: validates, sends, toasts and closes", async () => {
    const user = setupUser();
    vi.mocked(createAddressAction).mockResolvedValue({ ok: true });
    const onClose = vi.fn();
    render(<AddressModal onClose={onClose} />);

    await user.click(screen.getByRole("button", { name: "Thêm mới" }));
    expect(createAddressAction).not.toHaveBeenCalled();
    expect(screen.getByText("Vui lòng nhập họ và tên.")).toBeVisible();
    expect(screen.getByLabelText(/Họ và tên/)).toHaveFocus();

    await fillRequired(user);
    await user.click(screen.getByRole("button", { name: "Thêm mới" }));

    expect(createAddressAction).toHaveBeenCalledTimes(1);
    const sent = vi.mocked(createAddressAction).mock.calls[0][0];
    expect(sent.get("recipientName")).toBe("Nguyễn Văn A");
    expect(sent.get("city")).toBe("TP. HCM");
    expect(toast.success).toHaveBeenCalledWith("Đã thêm địa chỉ thành công.");
    expect(onClose).toHaveBeenCalled();
  });

  it("keeps the e2e field names", () => {
    render(<AddressModal onClose={() => {}} />);
    for (const name of [
      "recipientName",
      "phone",
      "street",
      "ward",
      "district",
      "city",
      "isDefault",
    ]) {
      expect(document.querySelector(`[name="${name}"]`)).not.toBeNull();
    }
  });

  it("closes on Escape and returns focus to the opener", async () => {
    const user = setupUser();
    render(<AddAddressButton />);
    const opener = screen.getByRole("button", { name: "Thêm địa chỉ mới" });
    await user.click(opener);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(opener).toHaveFocus();
  });

  it("shows an error toast and stays open when the action fails", async () => {
    const user = setupUser();
    vi.mocked(createAddressAction).mockResolvedValue({
      ok: false,
      error: "Địa chỉ trùng lặp.",
    });
    const onClose = vi.fn();
    render(<AddressModal onClose={onClose} />);
    await fillRequired(user);
    await user.click(screen.getByRole("button", { name: "Thêm mới" }));

    expect(toast.error).toHaveBeenCalledWith("Địa chỉ trùng lặp.");
    expect(screen.getByRole("alert")).toHaveTextContent("Địa chỉ trùng lặp.");
    expect(onClose).not.toHaveBeenCalled();
  });

  it("is pending while the action runs", async () => {
    const user = setupUser();
    let release: (v: { ok: true }) => void = () => {};
    vi.mocked(createAddressAction).mockReturnValue(
      new Promise((resolve) => {
        release = resolve;
      }),
    );
    render(<AddressModal onClose={() => {}} />);
    await fillRequired(user);
    await user.click(screen.getByRole("button", { name: "Thêm mới" }));
    const submit = screen.getByRole("button", { name: "Thêm mới" });
    expect(submit).toHaveAttribute("aria-busy", "true");
    expect(submit).toBeDisabled();
    release({ ok: true });
  });
});

function Harness() {
  const [present, setPresent] = useState(true);
  return present ? (
    <AddressActions address={office} />
  ) : (
    <p>đã xóa khỏi danh sách</p>
  );
}

describe("AddressActions", () => {
  it("delete asks for confirmation naming the recipient", async () => {
    const user = setupUser();
    render(<AddressActions address={office} />);
    await user.click(screen.getByRole("button", { name: "Xóa" }));
    const dialog = screen.getByRole("dialog", { name: "Xóa địa chỉ?" });
    expect(dialog).toHaveTextContent("Trần Thị B");
    expect(deleteAddressAction).not.toHaveBeenCalled();
  });

  it("cancelling with Escape leaves the list unchanged and refocuses Xóa", async () => {
    const user = setupUser();
    render(<Harness />);
    const remove = screen.getByRole("button", { name: "Xóa" });
    await user.click(remove);
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(deleteAddressAction).not.toHaveBeenCalled();
    expect(remove).toHaveFocus();
  });

  it("deletes only after the confirm button is pressed", async () => {
    const user = setupUser();
    vi.mocked(deleteAddressAction).mockResolvedValue({ ok: true });
    render(<AddressActions address={office} />);
    await user.click(screen.getByRole("button", { name: "Xóa" }));
    await user.click(screen.getByRole("button", { name: "Xóa địa chỉ" }));
    expect(deleteAddressAction).toHaveBeenCalledWith("a2");
    expect(toast.success).toHaveBeenCalledWith("Đã xóa địa chỉ.");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("a failed set-default shows an error toast and re-enables the button", async () => {
    const user = setupUser();
    vi.mocked(setDefaultAddressAction).mockResolvedValue({
      ok: false,
      error: "Không thể đặt mặc định.",
    });
    render(<AddressActions address={office} />);
    const button = screen.getByRole("button", { name: "Thiết lập mặc định" });
    await user.click(button);
    expect(toast.error).toHaveBeenCalledWith("Không thể đặt mặc định.");
    expect(
      screen.getByRole("button", { name: "Thiết lập mặc định" }),
    ).toBeEnabled();
  });

  it("a failed delete keeps the confirm open with an error toast", async () => {
    const user = setupUser();
    vi.mocked(deleteAddressAction).mockResolvedValue({
      ok: false,
      error: "Xóa thất bại.",
    });
    render(<AddressActions address={office} />);
    await user.click(screen.getByRole("button", { name: "Xóa" }));
    await user.click(screen.getByRole("button", { name: "Xóa địa chỉ" }));
    expect(toast.error).toHaveBeenCalledWith("Xóa thất bại.");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});
