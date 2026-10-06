import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ViewAddress } from "@/lib/gateway/addresses";
import { setupUser } from "@/test/user";

import { AddressSelectorModal } from "./AddressSelectorModal";

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  showToast: vi.fn(),
}));
const nav = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: nav.replace }),
  usePathname: () => "/checkout",
  useSearchParams: () => new URLSearchParams("step=address"),
}));
// The existing add-address form is reused unchanged; stub it to a save button.
vi.mock("./AddressModal", () => ({
  AddressModal: ({ onClose }: { onClose: () => void }) => (
    <button type="button" onClick={onClose}>
      stub-save-address
    </button>
  ),
}));

function addr(id: string, name: string, isDefault = false): ViewAddress {
  return {
    id,
    userId: "u1",
    recipientName: name,
    phone: "0900000000",
    street: "1 Đường A",
    ward: "P1",
    district: "Q1",
    city: "Hồ Chí Minh",
    isDefault,
  };
}

const list = [addr("a1", "Nguyễn Văn A", true), addr("a2", "Trần Thị B")];

beforeEach(() => vi.clearAllMocks());

describe("AddressSelectorModal", () => {
  it("shows the selected address with a Mặc định tag", () => {
    render(<AddressSelectorModal addresses={list} selectedId="a1" />);
    expect(screen.getByText("Nguyễn Văn A")).toBeInTheDocument();
    expect(screen.getByText("Mặc định")).toBeInTheDocument();
    expect(
      screen.getByText("1 Đường A, P1, Q1, Hồ Chí Minh"),
    ).toBeInTheDocument();
  });

  it("changing the address updates ?addr= and closes the modal", async () => {
    const user = setupUser();
    render(<AddressSelectorModal addresses={list} selectedId="a1" />);
    await user.click(screen.getByRole("button", { name: "Thay đổi" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await user.click(screen.getByRole("radio", { name: /Trần Thị B/ }));
    await user.click(screen.getByRole("button", { name: "Xác nhận" }));
    expect(nav.replace).toHaveBeenCalledWith("/checkout?step=address&addr=a2");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("adding an address selects the new one and shows a success toast", async () => {
    const user = setupUser();
    const { rerender } = render(
      <AddressSelectorModal addresses={list} selectedId="a1" />,
    );
    await user.click(screen.getByRole("button", { name: "Thay đổi" }));
    await user.click(screen.getByRole("button", { name: "Thêm địa chỉ mới" }));
    await user.click(screen.getByRole("button", { name: "stub-save-address" }));

    // the server action revalidated /checkout: the list now has a3
    rerender(
      <AddressSelectorModal
        addresses={[...list, addr("a3", "Lê Văn C")]}
        selectedId="a1"
      />,
    );
    expect(nav.replace).toHaveBeenCalledWith("/checkout?step=address&addr=a3");
    expect(toast.success).toHaveBeenCalledWith("Đã thêm địa chỉ mới.");
  });

  it("with no addresses shows an Empty with a Thêm địa chỉ action and no Thay đổi", async () => {
    const user = setupUser();
    render(<AddressSelectorModal addresses={[]} />);
    expect(
      screen.getByText("Bạn chưa có địa chỉ nhận hàng."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Thay đổi" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Thêm địa chỉ" }));
    expect(
      screen.getByRole("button", { name: "stub-save-address" }),
    ).toBeInTheDocument();
  });
});
