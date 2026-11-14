import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  addBundleToCart,
  addToCart,
  clearCart,
  removeFromCart,
  updateCartItem,
} from "@/lib/gateway/cart";

import {
  addBundleToCartAction,
  addToCartAction,
  clearCartAction,
  removeFromCartAction,
  updateCartItemAction,
} from "./actions";

vi.mock("@/lib/gateway/cart", () => ({
  addToCart: vi.fn(),
  addBundleToCart: vi.fn(),
  updateCartItem: vi.fn(),
  removeFromCart: vi.fn(),
  clearCart: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

function line(id: string, listingId: string, variantId: string) {
  return {
    id,
    listingId,
    variantId,
    quantity: 1,
    unitPrice: 1000,
    title: "t",
    variantName: "",
    imageUrl: "",
    sellerId: "s",
    sellerDisplayName: "",
  };
}

function cartOf(...items: ReturnType<typeof line>[]) {
  return { userId: "u", items, subtotal: 0, totalItems: items.length };
}

describe("addToCartAction", () => {
  it("adds to cart, revalidates, and returns success", async () => {
    vi.mocked(addToCart).mockResolvedValue(
      cartOf(line("ci1", "l1", "v1")) as never,
    );
    const res = await addToCartAction("l1", "v1", 3);
    expect(addToCart).toHaveBeenCalledWith("l1", "v1", 3);
    expect(revalidatePath).toHaveBeenCalledWith("/cart");
    expect(res.ok).toBe(true);
  });

  it("Cart selection: the action returns the matching line id - returns the id of the added line", async () => {
    vi.mocked(addToCart).mockResolvedValue(
      cartOf(line("ci-w", "L", "W"), line("ci-v", "L", "V")) as never,
    );
    const res = await addToCartAction("L", "V", 1);
    expect(res).toEqual({ ok: true, data: { itemId: "ci-v" } });
  });

  it("Cart selection: a listing without variants matches the empty variant id - matches a line without a variant", async () => {
    vi.mocked(addToCart).mockResolvedValue(
      cartOf(line("ci-other", "M", ""), line("ci-l", "L", "")) as never,
    );
    const res = await addToCartAction("L", undefined, 1);
    expect(res).toEqual({ ok: true, data: { itemId: "ci-l" } });
  });

  it("Cart selection: Mua ngay does not fall back to the whole cart - no matching line is ok:false", async () => {
    vi.mocked(addToCart).mockResolvedValue(
      cartOf(line("ci-w", "L", "W"), line("ci-m", "M", "V")) as never,
    );
    const res = await addToCartAction("L", "V", 1);
    expect(res.ok).toBe(false);
    expect(typeof (res as { error?: unknown }).error).toBe("string");
    expect((res as { error: string }).error).not.toBe("");
    expect(res).not.toHaveProperty("unauthenticated");
  });

  it("Cart selection: no matching line in an empty cart is ok:false", async () => {
    vi.mocked(addToCart).mockResolvedValue(cartOf() as never);
    const res = await addToCartAction("L", undefined, 1);
    expect(res.ok).toBe(false);
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

describe("addBundleToCartAction", () => {
  it("C1 adds the bundle, revalidates /cart and /checkout and returns success", async () => {
    vi.mocked(addBundleToCart).mockResolvedValue(cartOf() as never);
    const res = await addBundleToCartAction("b1");
    expect(addBundleToCart).toHaveBeenCalledWith("b1", 1);
    expect(revalidatePath).toHaveBeenCalledWith("/cart");
    expect(revalidatePath).toHaveBeenCalledWith("/checkout");
    expect(res.ok).toBe(true);
  });

  it("C4 addBundleToCartAction reports unauthenticated", async () => {
    vi.mocked(addBundleToCart).mockRejectedValue(
      new ConnectError("no session", Code.Unauthenticated),
    );
    const res = await addBundleToCartAction("b1");
    expect(res).toMatchObject({ ok: false, unauthenticated: true });
    expect(typeof (res as { error?: unknown }).error).toBe("string");
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("C3 failed precondition maps to unavailable", async () => {
    vi.mocked(addBundleToCart).mockRejectedValue(
      new ConnectError("listing unpublished", Code.FailedPrecondition),
    );
    const res = await addBundleToCartAction("b1");
    expect(res).toMatchObject({ ok: false, unavailable: true });
    expect(res).not.toHaveProperty("unauthenticated");
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("C6 addBundleToCartAction maps not found and outage to a plain error", async () => {
    for (const code of [Code.NotFound, Code.Unavailable]) {
      vi.mocked(addBundleToCart).mockRejectedValue(
        new ConnectError("boom", code),
      );
      const res = await addBundleToCartAction("b1");
      expect(res.ok).toBe(false);
      expect(typeof (res as { error?: unknown }).error).toBe("string");
      expect((res as { error: string }).error).not.toBe("");
      expect(res).not.toHaveProperty("unavailable");
      expect(res).not.toHaveProperty("unauthenticated");
    }
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe("bundleDropped on remove and update (C10)", () => {
  it("C10 removeFromCartAction reports bundleDropped for a bundled line", async () => {
    vi.mocked(removeFromCart).mockResolvedValue({} as never);
    const res = await removeFromCartAction("ci1", "b1");
    expect(removeFromCart).toHaveBeenCalledWith("ci1");
    expect(res).toEqual({ ok: true, bundleDropped: true });
  });

  it("C10 updateCartItemAction reports bundleDropped for a bundled line", async () => {
    vi.mocked(updateCartItem).mockResolvedValue({} as never);
    const res = await updateCartItemAction("ci1", 3, "b1");
    expect(updateCartItem).toHaveBeenCalledWith("ci1", 3);
    expect(res).toEqual({ ok: true, bundleDropped: true });
  });

  it("C10 a regular line never reports bundleDropped", async () => {
    vi.mocked(removeFromCart).mockResolvedValue({} as never);
    vi.mocked(updateCartItem).mockResolvedValue({} as never);
    expect(await removeFromCartAction("ci1", "")).toEqual({ ok: true });
    expect(await updateCartItemAction("ci1", 2, "")).toEqual({ ok: true });
  });

  it("C10 a failed remove of a bundled line does not report bundleDropped", async () => {
    vi.mocked(removeFromCart).mockRejectedValue(new Error("gone"));
    const res = await removeFromCartAction("ci1", "b1");
    expect(res).toEqual({ ok: false, error: "gone" });
  });
});
