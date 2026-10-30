import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ShopProfileForm } from "./ShopProfileForm";
import { upsertStorefrontAction } from "./actions";

vi.mock("./actions", () => ({ upsertStorefrontAction: vi.fn() }));
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
}));

const field = () => screen.getByLabelText(/Tên gian hàng/);
const save = () =>
  fireEvent.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

beforeEach(() => vi.clearAllMocks());

describe("ShopProfileForm", () => {
  it("is pre-filled with the current name", () => {
    render(<ShopProfileForm initialName="Cửa hàng Hoa Mai" />);
    expect(field()).toHaveValue("Cửa hàng Hoa Mai");
  });

  it("saves a new name: pending, then a success toast", async () => {
    let resolve: (v: unknown) => void = () => {};
    vi.mocked(upsertStorefrontAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }) as never,
    );
    render(<ShopProfileForm initialName="" />);
    fireEvent.change(field(), { target: { value: "Nhà Sách An Nhiên" } });
    save();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Lưu thay đổi" }),
      ).toBeDisabled(),
    );
    expect(
      screen.getByRole("button", { name: "Lưu thay đổi" }),
    ).toHaveAttribute("aria-busy", "true");
    expect(upsertStorefrontAction).toHaveBeenCalledWith("Nhà Sách An Nhiên");
    resolve({ ok: true, data: { displayName: "Nhà Sách An Nhiên" } });
    await waitFor(() =>
      expect(toastSuccess).toHaveBeenCalledWith("Đã lưu tên gian hàng"),
    );
  });

  it("rejects a blank name inline and sends nothing", () => {
    render(<ShopProfileForm initialName="Cũ" />);
    fireEvent.change(field(), { target: { value: "   " } });
    save();
    expect(field()).toHaveAttribute("aria-invalid", "true");
    const describedBy = field().getAttribute("aria-describedby") ?? "";
    expect(document.getElementById(describedBy)).toHaveTextContent(
      "Nhập tên gian hàng.",
    );
    expect(upsertStorefrontAction).not.toHaveBeenCalled();
  });

  it("rejects a name longer than 80 characters", () => {
    render(<ShopProfileForm initialName="" />);
    fireEvent.change(field(), { target: { value: "x".repeat(81) } });
    save();
    expect(
      screen.getByText("Tên gian hàng tối đa 80 ký tự."),
    ).toBeInTheDocument();
    expect(upsertStorefrontAction).not.toHaveBeenCalled();
  });

  it("accepts exactly 80 characters", async () => {
    vi.mocked(upsertStorefrontAction).mockResolvedValue({ ok: true });
    render(<ShopProfileForm initialName="" />);
    fireEvent.change(field(), { target: { value: "x".repeat(80) } });
    save();
    await waitFor(() => expect(upsertStorefrontAction).toHaveBeenCalled());
  });

  it("shows a server error in an Alert and a toast", async () => {
    vi.mocked(upsertStorefrontAction).mockResolvedValue({
      ok: false,
      error: "slug taken",
    });
    render(<ShopProfileForm initialName="A" />);
    save();
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("slug taken"),
    );
    expect(toastError).toHaveBeenCalledWith("slug taken");
  });
});
