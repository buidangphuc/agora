import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { deriveGettingStarted } from "./gettingStarted";

type Sources = Parameters<typeof deriveGettingStarted>[0];

const NEVER = new Promise<never>(() => {});

const FULL_STOREFRONT = { displayName: "Shop An", bannerUrl: "https://cdn/b.png" };
const EMPTY_STOREFRONT = { displayName: "", bannerUrl: "" };
const ONE = { ok: true as const, items: [{ id: "x" }], nextCursor: "" };
const NONE = { ok: true as const, items: [], nextCursor: "" };
const DOWN = { ok: false as const, code: "unavailable", message: "down" };

function sources(over: Partial<Record<keyof Sources, unknown>> = {}): Sources {
  const base = {
    storefront: async () => EMPTY_STOREFRONT,
    listings: async () => ({ items: [], nextCursor: "", total: 0 }),
    vouchers: async () => NONE,
    campaigns: async () => NONE,
  };
  return { ...base, ...over } as unknown as Sources;
}

function states(r: Awaited<ReturnType<typeof deriveGettingStarted>>) {
  return r.steps.map((s) => s.state);
}

beforeEach(() => {
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
});

describe("deriveGettingStarted", () => {
  it("all steps not done for an empty shop", async () => {
    const r = await deriveGettingStarted(sources());
    expect(r.steps).toHaveLength(4);
    expect(states(r)).toEqual(["todo", "todo", "todo", "todo"]);
    expect(r.steps.map((s) => s.href)).toEqual([
      "/seller/shop",
      "/seller/shop",
      "/seller/new",
      "/seller/marketing",
    ]);
    expect(r.steps.map((s) => s.label)).toEqual([
      "Đặt tên gian hàng",
      "Thêm ảnh bìa gian hàng",
      "Đăng sản phẩm đầu tiên",
      "Tạo voucher hoặc flash sale đầu tiên",
    ]);
    expect(JSON.stringify(r)).not.toMatch(/kyc/i);
    expect(r.allDone).toBe(false);
  });

  it("marks steps from storefront, listing and promotion data", async () => {
    const r = await deriveGettingStarted(
      sources({
        storefront: async () => ({ displayName: "Shop An", bannerUrl: "" }),
        listings: async () => ({ items: [{ id: "l1" }], nextCursor: "", total: 1 }),
        campaigns: async () => ONE,
      }),
    );
    expect(states(r)).toEqual(["done", "todo", "done", "done"]);
    expect(r.allDone).toBe(false);
  });

  it("allDone is true only when all four steps are done", async () => {
    const r = await deriveGettingStarted(
      sources({
        storefront: async () => FULL_STOREFRONT,
        listings: async () => ({ items: [{ id: "l1" }], nextCursor: "", total: 1 }),
        vouchers: async () => ONE,
      }),
    );
    expect(states(r)).toEqual(["done", "done", "done", "done"]);
    expect(r.allDone).toBe(true);
  });

  it("a voucher of any state completes step 4", async () => {
    const r = await deriveGettingStarted(
      sources({
        vouchers: async () => ({
          ok: true,
          items: [{ id: "v1", state: "deactivated", active: false }],
          nextCursor: "",
        }),
      }),
    );
    expect(r.steps[3].state).toBe("done");
  });

  it("missing storefront is not done not unknown", async () => {
    const r = await deriveGettingStarted(
      sources({
        storefront: async () => null,
        listings: async () => ({ items: [{ id: "l1" }], nextCursor: "", total: 1 }),
        campaigns: async () => ONE,
      }),
    );
    expect(states(r)).toEqual(["todo", "todo", "done", "done"]);
  });

  it("step 4 unknown when one source fails and the other is empty", async () => {
    const r1 = await deriveGettingStarted(
      sources({ vouchers: async () => DOWN, campaigns: async () => NONE }),
    );
    expect(r1.steps[3].state).toBe("unknown");
    expect(r1.allDone).toBe(false);

    const r2 = await deriveGettingStarted(
      sources({
        vouchers: async () => NONE,
        campaigns: async () => {
          throw new Error("boom");
        },
      }),
    );
    expect(r2.steps[3].state).toBe("unknown");
  });

  it("step 4 done when one source has a row though the other fails", async () => {
    const r = await deriveGettingStarted(
      sources({ vouchers: async () => DOWN, campaigns: async () => ONE }),
    );
    expect(r.steps[3].state).toBe("done");
  });

  it("a failing listings source makes only step 3 unknown", async () => {
    const r = await deriveGettingStarted(
      sources({
        storefront: async () => FULL_STOREFRONT,
        listings: async () => {
          throw new Error("unavailable");
        },
        vouchers: async () => ONE,
      }),
    );
    expect(states(r)).toEqual(["done", "done", "unknown", "done"]);
    expect(r.allDone).toBe(false);
  });

  it("a failing storefront source makes steps 1 and 2 unknown", async () => {
    const r = await deriveGettingStarted(
      sources({
        storefront: async () => {
          throw new Error("unavailable");
        },
      }),
    );
    expect(states(r)).toEqual(["unknown", "unknown", "todo", "todo"]);
  });

  it("a source slower than 3s becomes unknown", async () => {
    let settled = false;
    const p = deriveGettingStarted(sources({ storefront: () => NEVER })).then(
      (r) => {
        settled = true;
        return r;
      },
    );
    await vi.advanceTimersByTimeAsync(2999);
    expect(settled).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    const r = await p;
    expect(settled).toBe(true);
    expect(states(r)).toEqual(["unknown", "unknown", "todo", "todo"]);
  });

  it("sources run in parallel each capped at 3s", async () => {
    const p = deriveGettingStarted(
      sources({ storefront: () => NEVER, listings: () => NEVER, vouchers: () => NEVER }),
    );
    await vi.advanceTimersByTimeAsync(3000);
    const r = await p;
    expect(states(r)).toEqual(["unknown", "unknown", "unknown", "todo"]);
  });
});
