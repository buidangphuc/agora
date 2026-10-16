import { Code, ConnectError } from "@connectrpc/connect";
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
    expect(res).toEqual({ ok: true });
  });

  it("returns the error message when the gateway throws", async () => {
    vi.mocked(addToCart).mockRejectedValue(new Error("out of stock"));
    const res = await addToCartAction("l1");
    expect(res).toEqual({ ok: false, error: "out of stock" });
  });
});

describe("addToCartAction unauthenticated (login-return: guest add-to-cart)", () => {
  it("guest add-to-cart: Unauthenticated resolves ok:false with unauthenticated:true", async () => {
    vi.mocked(addToCart).mockRejectedValue(
      new ConnectError("no session", Code.Unauthenticated),
    );
    const res = await addToCartAction("l1", "v1", 1);
    expect(res).toMatchObject({ ok: false, unauthenticated: true });
    expect(typeof (res as { error?: unknown }).error).toBe("string");
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("non-auth errors stay generic: FailedPrecondition has no unauthenticated flag", async () => {
    vi.mocked(addToCart).mockRejectedValue(
      new ConnectError("out of stock", Code.FailedPrecondition),
    );
    const res = await addToCartAction("l1", "v1", 1);
    expect(res).toMatchObject({ ok: false });
    expect(res).not.toHaveProperty("unauthenticated");
  });

  it("non-auth errors stay generic: a plain Error has no unauthenticated flag", async () => {
    vi.mocked(addToCart).mockRejectedValue(new Error("boom"));
    const res = await addToCartAction("l1");
    expect(res).toEqual({ ok: false, error: "boom" });
  });

  it("other actions keep the shared contract: updateCartItemAction Unauthenticated has no flag", async () => {
    vi.mocked(updateCartItem).mockRejectedValue(
      new ConnectError("no session", Code.Unauthenticated),
    );
    const res = await updateCartItemAction("ci1", 2);
    expect(res).not.toHaveProperty("unauthenticated");
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
    expect(res).toEqual({ ok: false, error: "gone" });
  });

  it("updateCartItemAction returns the fallback error for a non-Error throw", async () => {
    vi.mocked(updateCartItem).mockRejectedValue("boom");
    const res = await updateCartItemAction("ci1", 2);
    expect(res).toEqual({ ok: false, error: "Cập nhật thất bại." });
  });

  it("clearCartAction clears and revalidates", async () => {
    vi.mocked(clearCart).mockResolvedValue(undefined);
    const res = await clearCartAction();
    expect(clearCart).toHaveBeenCalled();
    expect(res).toEqual({ ok: true });
  });
});
