import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("pushDataLayer", () => {
  beforeEach(() => {
    vi.resetModules();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("[dataLayer receives ecommerce items] view_item_list with 3 items is preceded by { ecommerce: null }", async () => {
    const win: { dataLayer?: Record<string, unknown>[] } = {};
    vi.stubGlobal("window", win);
    const { pushDataLayer } = await import("./dataLayer");

    pushDataLayer("view_item_list", {
      currency: "VND",
      items: [
        { itemId: "a", price: 1, index: 1 },
        { itemId: "b", price: 2, index: 2 },
        { itemId: "c", price: 3, index: 3 },
      ],
    });

    const dl = win.dataLayer!;
    const i = dl.findIndex((e) => e.event === "view_item_list");
    expect(i).toBeGreaterThan(0);
    expect(dl[i - 1]).toEqual({ ecommerce: null });
    expect((dl[i].ecommerce as { items: unknown[] }).items).toHaveLength(3);
  });

  it("[dataLayer receives ecommerce items] every ecommerce push is preceded by a reset, even back to back", async () => {
    const win: { dataLayer?: Record<string, unknown>[] } = {};
    vi.stubGlobal("window", win);
    const { pushDataLayer } = await import("./dataLayer");

    pushDataLayer("view_item", { items: [{ itemId: "a" }] });
    pushDataLayer("add_to_cart", { items: [{ itemId: "a" }] });

    const dl = win.dataLayer!;
    const events = dl
      .map((e, idx) => ({ e, idx }))
      .filter(({ e }) => typeof e.event === "string");
    expect(events).toHaveLength(2);
    for (const { idx } of events)
      expect(dl[idx - 1]).toEqual({ ecommerce: null });
  });

  it("[Dispatch during SSR is a no-op] pushDataLayer without window returns without throwing", async () => {
    vi.stubGlobal("window", undefined);
    const { pushDataLayer } = await import("./dataLayer");
    expect(() =>
      pushDataLayer("view_item", { items: [{ itemId: "a" }] }),
    ).not.toThrow();
  });
});
