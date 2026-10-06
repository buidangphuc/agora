import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";

import { CartQuantityControl } from "./CartQuantityControl";
import { removeFromCartAction, updateCartItemAction } from "./actions";

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  showToast: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));
vi.mock("./actions", () => ({
  updateCartItemAction: vi.fn(),
  removeFromCartAction: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

describe("CartQuantityControl", () => {
  it("disables picker and remove with aria-busy while an update is pending", async () => {
    const user = setupUser();
    const d = deferred<{ ok: true }>();
    vi.mocked(updateCartItemAction).mockReturnValue(d.promise);
    const { container } = render(
      <CartQuantityControl itemId="ci1" quantity={1} title="Áo" />,
    );
    await user.click(screen.getByRole("button", { name: "Tăng số lượng" }));

    expect(updateCartItemAction).toHaveBeenCalledWith("ci1", 2);
    expect(container.firstElementChild).toHaveAttribute("aria-busy", "true");
    expect(
      screen.getByRole("button", { name: "Tăng số lượng" }),
    ).toBeDisabled();
    expect(screen.getByRole("button", { name: "Xóa Áo" })).toBeDisabled();

    await act(async () => {
      d.resolve({ ok: true });
      await d.promise;
    });
    expect(screen.getByRole("button", { name: "Tăng số lượng" })).toBeEnabled();
    expect(container.firstElementChild).not.toHaveAttribute("aria-busy");
  });

  it("reports a failed update with an error toast and keeps the server quantity", async () => {
    const user = setupUser();
    vi.mocked(updateCartItemAction).mockResolvedValue({
      ok: false,
      error: "Không đủ tồn kho",
    });
    render(<CartQuantityControl itemId="ci1" quantity={2} title="Áo" />);
    await user.click(screen.getByRole("button", { name: "Tăng số lượng" }));

    expect(toast.error).toHaveBeenCalledWith("Không đủ tồn kho");
    expect(screen.getByRole("spinbutton")).toHaveValue("2");
    expect(screen.getByRole("button", { name: "Tăng số lượng" })).toBeEnabled();
  });

  it("cannot go below the minimum quantity and calls no action", async () => {
    const user = setupUser();
    render(<CartQuantityControl itemId="ci1" quantity={1} title="Áo" />);
    const dec = screen.getByRole("button", { name: "Giảm số lượng" });
    expect(dec).toBeDisabled();
    await user.click(dec);
    expect(updateCartItemAction).not.toHaveBeenCalled();
  });

  it("removes the item and shows an info toast", async () => {
    const user = setupUser();
    vi.mocked(removeFromCartAction).mockResolvedValue({ ok: true });
    render(<CartQuantityControl itemId="ci1" quantity={2} title="Áo" />);
    await user.click(screen.getByRole("button", { name: "Xóa Áo" }));
    expect(removeFromCartAction).toHaveBeenCalledWith("ci1");
    expect(toast.info).toHaveBeenCalledWith("Đã xóa sản phẩm khỏi giỏ hàng.");
  });

  it("reports a thrown action as an error toast and re-enables the controls", async () => {
    const user = setupUser();
    vi.mocked(updateCartItemAction).mockRejectedValue(new Error("network"));
    render(<CartQuantityControl itemId="ci1" quantity={2} title="Áo" />);
    await user.click(screen.getByRole("button", { name: "Tăng số lượng" }));
    expect(toast.error).toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Tăng số lượng" })).toBeEnabled();
  });
});
