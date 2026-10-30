import { ApiError, PaymentMethod } from "@fe-platform/api-client";
import { beforeEach, describe, expect, it, vi } from "vitest";

const clients = {
  cart: {
    addToCart: vi.fn(),
    updateCartItem: vi.fn(),
    removeFromCart: vi.fn(),
  },
  address: { createAddress: vi.fn() },
  order: { createOrder: vi.fn() },
};

vi.mock("@fe-platform/api-client", async (orig) => ({
  ...(await orig<typeof import("@fe-platform/api-client")>()),
  createGatewayClients: () => clients,
}));
vi.mock("next/navigation", () => ({
  notFound: () => {
    throw new Error("NEXT_NOT_FOUND");
  },
  redirect: (to: string) => {
    throw new Error(`NEXT_REDIRECT:${to}`);
  },
}));
vi.mock("../../../lib/session", () => ({
  requireSession: async () => "tok",
  safeNext: (n: string) => n,
}));

import { addToCart } from "../products/[id]/actions";
import { removeItem, updateQuantity } from "./cart/actions";
import { createAddress, placeOrder } from "./checkout/actions";

const form = (o: Record<string, string>) => {
  const f = new FormData();
  for (const [k, v] of Object.entries(o)) f.set(k, v);
  return f;
};

beforeEach(() => {
  for (const g of Object.values(clients))
    for (const fn of Object.values(g)) fn.mockReset();
});

describe("addToCart", () => {
  it("adds one unit and goes to the cart", async () => {
    clients.cart.addToCart.mockResolvedValue({});
    await expect(
      addToCart(form({ listingId: "l1", variantId: "v1" })),
    ).rejects.toThrow("NEXT_REDIRECT:/shop/cart");
    expect(clients.cart.addToCart).toHaveBeenCalledWith({
      listingId: "l1",
      variantId: "v1",
      quantity: 1,
    });
  });

  it("returns to the product with an error code", async () => {
    clients.cart.addToCart.mockRejectedValue(
      new ApiError("invalid", "out of stock"),
    );
    await expect(addToCart(form({ listingId: "l1" }))).rejects.toThrow(
      "NEXT_REDIRECT:/products/l1?error=invalid",
    );
  });
});

describe("cart actions", () => {
  it("updates a quantity", async () => {
    clients.cart.updateCartItem.mockResolvedValue({});
    await expect(
      updateQuantity(form({ itemId: "i1", quantity: "3" })),
    ).rejects.toThrow("NEXT_REDIRECT:/shop/cart");
    expect(clients.cart.updateCartItem).toHaveBeenCalledWith({
      itemId: "i1",
      quantity: 3,
    });
  });

  it("rejects a non-positive quantity before calling the backend", async () => {
    await expect(
      updateQuantity(form({ itemId: "i1", quantity: "0" })),
    ).rejects.toThrow("NEXT_REDIRECT:/shop/cart?error=invalid");
    expect(clients.cart.updateCartItem).not.toHaveBeenCalled();
  });

  it("removes an item and reports a backend failure", async () => {
    clients.cart.removeFromCart.mockResolvedValue({});
    await expect(removeItem(form({ itemId: "i1" }))).rejects.toThrow(
      "NEXT_REDIRECT:/shop/cart",
    );
    clients.cart.removeFromCart.mockRejectedValue(
      new ApiError("unavailable", "down"),
    );
    await expect(removeItem(form({ itemId: "i1" }))).rejects.toThrow(
      "NEXT_REDIRECT:/shop/cart?error=unavailable",
    );
  });
});

describe("checkout actions", () => {
  it("creates an address and selects it", async () => {
    clients.address.createAddress.mockResolvedValue({ address: { id: "a9" } });
    await expect(
      createAddress(
        form({
          recipientName: "An",
          phone: "0900",
          street: "1 Main",
          city: "HCM",
          isDefault: "on",
        }),
      ),
    ).rejects.toThrow("NEXT_REDIRECT:/shop/checkout?address=a9");
    expect(clients.address.createAddress).toHaveBeenCalledWith(
      expect.objectContaining({ recipientName: "An", isDefault: true }),
    );
  });

  it("reports an address the backend refused", async () => {
    clients.address.createAddress.mockRejectedValue(
      new ApiError("invalid", "bad phone"),
    );
    await expect(createAddress(form({ recipientName: "An" }))).rejects.toThrow(
      "NEXT_REDIRECT:/shop/checkout?error=invalid",
    );
  });

  it("places the order for the selected address and goes to confirmation", async () => {
    clients.order.createOrder.mockResolvedValue({
      orders: [{ id: "o1" }, { id: "o2" }],
    });
    await expect(
      placeOrder(
        form({ addressId: "a1", paymentMethod: String(PaymentMethod.COD) }),
      ),
    ).rejects.toThrow("NEXT_REDIRECT:/shop/confirmation?order=o1%2Co2");
    expect(clients.order.createOrder).toHaveBeenCalledWith({
      addressId: "a1",
      paymentMethod: PaymentMethod.COD,
    });
  });

  it("keeps the address when the order is refused", async () => {
    clients.order.createOrder.mockRejectedValue(
      new ApiError("invalid", "cart is empty"),
    );
    await expect(placeOrder(form({ addressId: "a1" }))).rejects.toThrow(
      "NEXT_REDIRECT:/shop/checkout?address=a1&error=invalid",
    );
  });
});
