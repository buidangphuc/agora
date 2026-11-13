import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";

const push = vi.fn();
const addBundleToCartAction = vi.hoisted(() => vi.fn());

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, replace: vi.fn() }),
}));
vi.mock("@/features/cart/actions", () => ({ addBundleToCartAction }));

import { AddBundleButton } from "./AddBundleButton";

function deferred<T>() {
  let resolve: (v: T) => void = () => {};
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

const click = async () => {
  const user = setupUser();
  await user.click(screen.getByRole("button", { name: "Thêm combo vào giỏ" }));
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe("AddBundleButton", () => {
  it("[C5] sends one request on double click", async () => {
    const d = deferred<{ ok: true }>();
    addBundleToCartAction.mockReturnValue(d.promise);
    render(<AddBundleButton bundleId="b1" shopId="s1" />);
    const user = setupUser();
    const btn = screen.getByRole("button", { name: "Thêm combo vào giỏ" });
    await user.dblClick(btn);
    expect(addBundleToCartAction).toHaveBeenCalledTimes(1);
    expect(addBundleToCartAction).toHaveBeenCalledWith("b1");
    expect(btn).toBeDisabled();
    await act(async () => d.resolve({ ok: true }));
  });

  it("[C1] shows Đã thêm combo with a link to /cart on success", async () => {
    addBundleToCartAction.mockResolvedValue({ ok: true });
    render(<AddBundleButton bundleId="b1" shopId="s1" />);
    await click();
    expect(await screen.findByText(/Đã thêm combo/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /giỏ/i })).toHaveAttribute("href", "/cart");
  });

  it("[C3] flips to unavailable on failed precondition", async () => {
    addBundleToCartAction.mockResolvedValue({
      ok: false,
      error: "Combo hiện không khả dụng",
      unavailable: true,
    });
    render(<AddBundleButton bundleId="b1" shopId="s1" />);
    await click();
    expect(await screen.findByText("Combo tạm không khả dụng")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Thêm combo vào giỏ" })).toBeNull();
    expect(screen.queryByText(/Đã thêm combo/)).toBeNull();
  });

  it("[C6] shows a role=alert message on error and stays interactive", async () => {
    addBundleToCartAction.mockResolvedValue({
      ok: false,
      error: "Thêm combo vào giỏ hàng thất bại.",
    });
    render(<AddBundleButton bundleId="b1" shopId="s1" />);
    await click();
    expect(await screen.findByRole("alert")).toHaveTextContent("Thêm combo vào giỏ hàng thất bại.");
    expect(screen.queryByText(/Đã thêm combo/)).toBeNull();
    expect(screen.getByRole("button", { name: "Thêm combo vào giỏ" })).toBeEnabled();
  });

  it("[C4] sends a guest to login with a return path to the shop", async () => {
    addBundleToCartAction.mockResolvedValue({
      ok: false,
      error: "Chưa đăng nhập",
      unauthenticated: true,
    });
    render(<AddBundleButton bundleId="b1" shopId="s1" />);
    await click();
    await vi.waitFor(() => expect(push).toHaveBeenCalledTimes(1));
    const target = String(push.mock.calls[0][0]);
    expect(target).toContain("/login");
    expect(decodeURIComponent(target)).toContain("/shop/s1");
    expect(screen.queryByText(/Đã thêm combo/)).toBeNull();
  });
});
