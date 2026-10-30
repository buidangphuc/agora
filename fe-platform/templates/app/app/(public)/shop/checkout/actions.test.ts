import { beforeEach, describe, expect, it, vi } from "vitest";

const set = vi.fn();
vi.mock("next/headers", () => ({ cookies: async () => ({ set }) }));
vi.mock("next/navigation", () => ({
  redirect: (to: string) => {
    throw new Error(`NEXT_REDIRECT:${to}`);
  },
}));

import { placeOrder } from "./actions";

function form(values: Record<string, string>) {
  const f = new FormData();
  for (const [k, v] of Object.entries(values)) f.set(k, v);
  return f;
}

const good = {
  cart: "desk-lamp:2",
  name: "An Nguyen",
  address: "1 Main Street",
  city: "HCMC",
  phone: "0900000000",
};

beforeEach(() => set.mockReset());

describe("placeOrder", () => {
  it("stores delivery in an httpOnly cookie and redirects without personal data", async () => {
    const err = await placeOrder(form(good)).catch((e: Error) => e);
    expect((err as Error).message).toBe(
      "NEXT_REDIRECT:/shop/confirmation?cart=desk-lamp:2",
    );
    expect(set).toHaveBeenCalledOnce();
    const [name, , options] = set.mock.calls[0];
    expect(name).toBe("shop_delivery");
    expect(options).toMatchObject({
      httpOnly: true,
      sameSite: "lax",
      path: "/shop",
    });
    expect((err as Error).message).not.toContain("Nguyen");
    expect((err as Error).message).not.toContain("0900");
  });

  it("redirects back with field codes only when invalid", async () => {
    const err = await placeOrder(form({ ...good, phone: "1", city: "" })).catch(
      (e: Error) => e,
    );
    expect((err as Error).message).toBe(
      "NEXT_REDIRECT:/shop/checkout?cart=desk-lamp:2&error=city,phone",
    );
    expect(set).not.toHaveBeenCalled();
  });

  it("sends an empty cart back to the shop", async () => {
    const err = await placeOrder(form({ ...good, cart: "" })).catch(
      (e: Error) => e,
    );
    expect((err as Error).message).toBe("NEXT_REDIRECT:/shop");
  });
});
