import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  addToCart,
  clearCart,
  removeFromCart,
  updateCartItem,
} from "@/lib/gateway/cart";

import {
  addToCartAction,
  clearCartAction,
  removeFromCartAction,
  updateCartItemAction,
} from "./actions";

vi.mock("@/lib/gateway/cart", () => ({
  addToCart: vi.fn(),
  updateCartItem: vi.fn(),
  removeFromCart: vi.fn(),
  clearCart: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

describe("addToCartAction", () => {
  it("adds to cart, revalidates, and returns success", async () => {
    vi.mocked(addToCart).mockResolvedValue({} as never);
    const res = await addToCartAction("l1", "v1", 3);
    expect(addToCart).toHaveBeenCalledWith("l1", "v1", 3);
    expect(revalidatePath).toHaveBeenCalledWith("/cart");
    expect(res).toEqual({
      ok: true,
      message: "Đã thêm vào giỏ hàng thành công!",
    });
  });

  it("returns the error message when the gateway throws", async () => {
    vi.mocked(addToCart).mockRejectedValue(new Error("out of stock"));
    const res = await addToCartAction("l1");
    expect(res).toEqual({ ok: false, message: "out of stock" });
  });
});

describe("other cart actions", () => {
  it("updateCartItemAction forwards item id + quantity", async () => {
    vi.mocked(updateCartItem).mockResolvedValue({} as never);
    const res = await updateCartItemAction("ci1", 2);
    expect(updateCartItem).toHaveBeenCalledWith("ci1", 2);
    expect(res).toEqual({ ok: true });
  });

  it("removeFromCartAction returns error shape on failure", async () => {
    vi.mocked(removeFromCart).mockRejectedValue(new Error("gone"));
    const res = await removeFromCartAction("ci1");
    expect(res).toEqual({ ok: false, message: "gone" });
  });

  it("clearCartAction clears and revalidates", async () => {
    vi.mocked(clearCart).mockResolvedValue(undefined);
    const res = await clearCartAction();
    expect(clearCart).toHaveBeenCalled();
    expect(res).toEqual({ ok: true });
  });
});
