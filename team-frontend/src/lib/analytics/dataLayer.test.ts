import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("pushDataLayer", () => {
  beforeEach(() => {
    vi.resetModules();
    document.cookie = `agora_consent=v=1&a=1&m=0&p=0&ts=${Math.floor(Date.now() / 1000)}; Path=/`;
  });
  afterEach(() => {
    document.cookie = "agora_consent=; Max-Age=0; Path=/";
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

    const dl = win.dataLayer ?? [];
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

    const dl = win.dataLayer ?? [];
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

describe("dataLayer consent gating", () => {
  const clearCookie = () => {
    document.cookie = "agora_consent=; Max-Age=0; Path=/";
  };
  const setConsent = (a: 0 | 1) => {
    document.cookie = `agora_consent=v=1&a=${a}&m=0&p=0&ts=1700000000; Path=/`;
  };
  const ecommerceEvents = () =>
    (window.dataLayer ?? []).filter(
      (e) =>
        e &&
        typeof e === "object" &&
        !Array.isArray(e) &&
        typeof (e as Record<string, unknown>).event === "string" &&
        !String((e as Record<string, unknown>).event).startsWith("gtm."),
    );
  const items = [{ itemId: "a", price: 1, index: 1 }];

  beforeEach(() => {
    vi.resetModules();
    clearCookie();
    Reflect.deleteProperty(window, "dataLayer");
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", "GTM-TEST");
  });
  afterEach(() => {
    clearCookie();
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("[Revoking analytics stops pushes] after a revoke, trackEcommerce pushes no ecommerce event to window.dataLayer", async () => {
    const { applyConsent } = await import("./destinations/gtm");
    const { trackEcommerce } = await import("./dispatcher");
    setConsent(1);
    applyConsent({ analytics: true, marketing: false, personalization: false });
    trackEcommerce("view_item", { items });
    expect(ecommerceEvents()).toHaveLength(1);

    setConsent(0);
    applyConsent({
      analytics: false,
      marketing: false,
      personalization: false,
    });
    trackEcommerce("view_item_list", { currency: "VND", items });
    trackEcommerce("add_to_cart", { items });

    expect(ecommerceEvents()).toHaveLength(1);
    expect(ecommerceEvents()[0].event).toBe("view_item");
  });

  it("[No ecommerce push without analytics consent] no cookie and a=0: list/product views push no ecommerce event, consent default present", async () => {
    const { applyConsent } = await import("./destinations/gtm");
    const { trackEcommerce } = await import("./dispatcher");
    const none = {
      analytics: false,
      marketing: false,
      personalization: false,
    };
    for (const cookie of [null, 0] as const) {
      Reflect.deleteProperty(window, "dataLayer");
      clearCookie();
      if (cookie === 0) setConsent(0);
      applyConsent(none);
      trackEcommerce("view_item_list", { currency: "VND", items });
      trackEcommerce("view_item", { items });

      expect(ecommerceEvents()).toHaveLength(0);
      const hasDefault = ((window.dataLayer ?? []) as unknown[]).some((e) => {
        const a =
          e && typeof e === "object" && "length" in e
            ? Array.from(e as ArrayLike<unknown>)
            : null;
        return a?.[0] === "consent" && a[1] === "default";
      });
      expect(hasDefault).toBe(true);
    }
  });

  it("[Revoking analytics stops pushes] positive guard: with a=1, ecommerce events are pushed", async () => {
    const { trackEcommerce } = await import("./dispatcher");
    setConsent(1);
    trackEcommerce("view_item_list", { currency: "VND", items });
    trackEcommerce("view_item", { items });
    expect(ecommerceEvents().map((e) => e.event)).toEqual([
      "view_item_list",
      "view_item",
    ]);
  });
});

describe("dataLayer loop properties", () => {
  beforeEach(() => {
    vi.resetModules();
    Reflect.deleteProperty(window, "dataLayer");
    document.cookie = `agora_consent=v=1&a=1&m=0&p=0&ts=${Math.floor(Date.now() / 1000)}; Path=/`;
  });
  afterEach(() => {
    document.cookie = "agora_consent=; Max-Age=0; Path=/";
    vi.unstubAllGlobals();
  });

  it("passes loop properties and drops others", async () => {
    const { trackEcommerce } = await import("./dispatcher");
    trackEcommerce("sign_up", {
      properties: { method: "password", secret: "x" },
    });
    const entries = (window.dataLayer ?? []).filter(
      (e) => e.event === "sign_up",
    );
    expect(entries).toHaveLength(1);
    expect(entries[0].properties).toEqual({ method: "password" });
    expect(JSON.stringify(entries[0])).not.toContain("secret");
  });

  it("passes loop properties and drops others: every allowlisted key passes", async () => {
    const { pushDataLayer } = await import("./dataLayer");
    pushDataLayer("share", {
      properties: {
        method: "copy_link",
        content_type: "listing",
        seller_id: "S1",
        streak: "3",
        alert_type: "price_drop",
      },
    });
    const e = (window.dataLayer ?? []).find((x) => x.event === "share");
    expect(e?.properties).toEqual({
      method: "copy_link",
      content_type: "listing",
      seller_id: "S1",
      streak: "3",
      alert_type: "price_drop",
    });
  });
});
