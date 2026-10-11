import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type Entry = Record<string, unknown>;

async function setup() {
  const win: { dataLayer?: Entry[] } = {};
  vi.stubGlobal("window", win);
  const { pushDataLayer } = await import("./dataLayer");
  const last = () =>
    win.dataLayer!.filter((e) => typeof e.event === "string").at(-1)!;
  return { pushDataLayer, last };
}

describe("dataLayer privacy", () => {
  beforeEach(() => {
    vi.resetModules();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("[Caller properties cannot override reserved keys or leak to top level]", async () => {
    const { pushDataLayer, last } = await setup();
    pushDataLayer("view_item", {
      items: [{ itemId: "a" }],
      properties: {
        event: "purchase",
        ecommerce: { value: 1 } as unknown as string,
        email: "a@b.vn",
        note: "x",
      },
    });
    const e = last();
    expect(e.event).toBe("view_item");
    expect((e.ecommerce as { value?: number }).value).toBeUndefined();
    expect((e.ecommerce as { items: unknown[] }).items).toHaveLength(1);
    expect(e).not.toHaveProperty("email");
    expect(e).not.toHaveProperty("note");
    const props = (e.properties ?? {}) as Record<string, unknown>;
    expect(props).not.toHaveProperty("email");
    expect(props).not.toHaveProperty("event");
    expect(props).not.toHaveProperty("ecommerce");
    expect(JSON.stringify(e)).not.toContain("a@b.vn");
  });

  it("[Search term and path are scrubbed in the dataLayer]", async () => {
    const { pushDataLayer, last } = await setup();
    pushDataLayer("search_filter", {
      query: "iphone lan.nguyen@gmail.com 0912345678 1234567890123",
      path: "/search?q=0912345678",
    });
    const e = last();
    expect(e.search_term).toBe("iphone [email] [phone] [number]");
    expect(e.page_path).toBe("/search?q=[phone]");
    const s = JSON.stringify(e);
    expect(s).not.toContain("lan.nguyen@gmail.com");
    expect(s).not.toContain("0912345678");
    expect(s).not.toContain("1234567890123");
  });

  it("[Referrer is reduced to scheme host path]", async () => {
    const { pushDataLayer, last } = await setup();
    pushDataLayer("view_item_list", {
      referrer: "https://shop.example.vn/p/12?email=a@b.vn#x",
    });
    expect(last().page_referrer).toBe("https://shop.example.vn/p/12");
  });

  it("[Referrer is reduced to scheme host path] an unparseable referrer becomes empty", async () => {
    const { pushDataLayer, last } = await setup();
    pushDataLayer("view_item_list", { referrer: "not a url" });
    const e = last();
    expect(e.page_referrer ?? "").toBe("");
  });
});
