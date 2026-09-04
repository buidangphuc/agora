import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  addToCart,
  clearCart,
  getCart,
  removeFromCart,
  updateCartItem,
} from "./cart.js";
import { makeClients } from "./client.js";
import { getToken } from "./session.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

type CartRpcs = {
  getCart: ReturnType<typeof vi.fn>;
  addToCart: ReturnType<typeof vi.fn>;
  updateCartItem: ReturnType<typeof vi.fn>;
  removeFromCart: ReturnType<typeof vi.fn>;
  clearCart: ReturnType<typeof vi.fn>;
};

function stubCart(rpcs: Partial<CartRpcs> = {}) {
  const cart: CartRpcs = {
    getCart: vi.fn(),
    addToCart: vi.fn(),
    updateCartItem: vi.fn(),
    removeFromCart: vi.fn(),
    clearCart: vi.fn(),
    ...rpcs,
  };
  vi.mocked(makeClients).mockReturnValue({ cart } as never);
  return cart;
}

const sampleCart = {
  userId: "u1",
  subtotal: 150000n,
  items: [
    {
      id: "ci1",
      listingId: "l1",
      variantId: "v1",
      quantity: 2,
      unitPrice: 75000n,
      title: "Item",
      variantName: "Red",
      imageUrl: "img.png",
      sellerId: "s1",
    },
  ],
};

beforeEach(() => vi.clearAllMocks());

describe("cart gateway wrapper", () => {
  it("passes the session token to makeClients", async () => {
    stubCart({ getCart: vi.fn().mockResolvedValue({ cart: sampleCart }) });
    await getCart();
    expect(getToken).toHaveBeenCalled();
    expect(makeClients).toHaveBeenCalledWith("test-token");
  });

  it("getCart maps the proto cart to a view cart and derives totalItems", async () => {
    const cart = stubCart({
      getCart: vi.fn().mockResolvedValue({ cart: sampleCart }),
    });
    const view = await getCart();
    expect(cart.getCart).toHaveBeenCalledWith({});
    expect(view).toEqual({
      userId: "u1",
      subtotal: 150000,
      totalItems: 2,
      items: [
        {
          id: "ci1",
          listingId: "l1",
          variantId: "v1",
          quantity: 2,
          unitPrice: 75000,
          title: "Item",
          variantName: "Red",
          imageUrl: "img.png",
          sellerId: "s1",
        },
      ],
    });
  });

  it("getCart normalizes any RPC error to an empty cart", async () => {
    stubCart({ getCart: vi.fn().mockRejectedValue(new Error("boom")) });
    await expect(getCart()).resolves.toEqual({
      userId: "",
      items: [],
      subtotal: 0,
      totalItems: 0,
    });
  });

  it("addToCart maps args to the RPC request (defaulting variantId)", async () => {
    const cart = stubCart({
      addToCart: vi.fn().mockResolvedValue({ cart: sampleCart }),
    });
    await addToCart("l1");
    expect(cart.addToCart).toHaveBeenCalledWith({
      listingId: "l1",
      variantId: "",
      quantity: 1,
    });
  });

  it("updateCartItem and removeFromCart forward ids/quantities", async () => {
    const cart = stubCart({
      updateCartItem: vi.fn().mockResolvedValue({ cart: sampleCart }),
      removeFromCart: vi.fn().mockResolvedValue({ cart: sampleCart }),
    });
    await updateCartItem("ci1", 5);
    await removeFromCart("ci1");
    expect(cart.updateCartItem).toHaveBeenCalledWith({
      itemId: "ci1",
      quantity: 5,
    });
    expect(cart.removeFromCart).toHaveBeenCalledWith({ itemId: "ci1" });
  });

  it("clearCart invokes the clear RPC", async () => {
    const cart = stubCart({ clearCart: vi.fn().mockResolvedValue({}) });
    await clearCart();
    expect(cart.clearCart).toHaveBeenCalledWith({});
  });
});
