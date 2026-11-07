import { beforeEach, describe, expect, it, vi } from "vitest";

import { getStorefront } from "@/lib/gateway/listings";

vi.mock("@/lib/requestCache", () => ({
  requestCache: <A extends unknown[], R>(fn: (...a: A) => R) => {
    const memo = new Map<string, R>();
    return (...args: A): R => {
      const key = JSON.stringify(args);
      if (!memo.has(key)) memo.set(key, fn(...args));
      return memo.get(key) as R;
    };
  },
}));
vi.mock("@/features/shop/ShopStorefrontView", () => ({
  ShopStorefrontView: () => null,
}));
vi.mock("@/lib/gateway/reviews", () => ({
  getShopRatingSummary: vi.fn().mockResolvedValue(null),
}));
vi.mock("@/lib/gateway/engagement", () => ({ isFollowing: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn(() => null) }));
vi.mock("@/lib/gateway/search", () => ({
  searchListings: vi.fn().mockResolvedValue({ items: [] }),
}));
vi.mock("@/lib/gateway/listings", () => ({
  getStorefront: vi.fn(),
  listBundlesBySeller: vi.fn().mockResolvedValue([]),
}));

import ShopPage, { generateMetadata } from "./page";

let seq = 0;
function nextId() {
  seq += 1;
  return `seller-${seq}abcdef`;
}

function titleOf(md: { title?: unknown }): string | undefined {
  const t = md.title as
    | string
    | { absolute?: string; default?: string }
    | null
    | undefined;
  if (t == null) return undefined;
  return typeof t === "string" ? t : (t.absolute ?? t.default);
}

function imagesOf(md: { openGraph?: unknown }): unknown[] {
  const og = (md.openGraph ?? {}) as { images?: unknown };
  return ([] as unknown[]).concat(og.images ?? []);
}

async function metaFor(id: string) {
  return generateMetadata({ params: Promise.resolve({ id }) });
}

beforeEach(() => {
  vi.clearAllMocks();
  Reflect.deleteProperty(process.env, "NEXT_PUBLIC_SITE_URL");
});

describe("shop generateMetadata", () => {
  it("SEO-9 uses the display name, tagline, banner and canonical", async () => {
    const id = nextId();
    vi.mocked(getStorefront).mockResolvedValue({
      sellerId: id,
      displayName: "Cửa hàng Mai",
      tagline: "Hàng handmade",
      bannerUrl: "http://localhost:9000/listing-images/banner.jpg",
    } as never);
    const md = await metaFor(id);
    expect(titleOf(md)).toMatch(/^Cửa hàng Mai( \| Agora)?$/);
    expect(md.description).toBe("Hàng handmade");
    expect(String(md.alternates?.canonical)).toBe(
      `http://localhost:3000/shop/${id}`,
    );
    const og = md.openGraph as unknown as Record<string, unknown>;
    expect(og.type).toBe("website");
    expect(og.siteName).toBe("Agora");
    expect(String(og.url)).toBe(`http://localhost:3000/shop/${id}`);
    expect(
      imagesOf(md).map((i) =>
        typeof i === "string" ? i : String((i as { url: unknown }).url),
      ),
    ).toEqual(["http://localhost:9000/listing-images/banner.jpg"]);
  });

  it("SEO-9 an unset tagline gives the Cửa hàng ... trên Agora description", async () => {
    const id = nextId();
    vi.mocked(getStorefront).mockResolvedValue({
      sellerId: id,
      displayName: "Cửa hàng Mai",
      tagline: "",
      bannerUrl: "",
    } as never);
    const md = await metaFor(id);
    expect(md.description).toBe("Cửa hàng Cửa hàng Mai trên Agora");
    expect(imagesOf(md)).toEqual([]);
  });

  it("SEO-10 storefront failure falls back", async () => {
    const id = nextId();
    vi.mocked(getStorefront).mockRejectedValue(new Error("gateway down"));
    const md = await metaFor(id);
    expect(titleOf(md)).toMatch(
      new RegExp(`^Shop #${id.slice(0, 6)}( \\| Agora)?$`),
    );
    expect(imagesOf(md)).toEqual([]);
    expect(String(md.alternates?.canonical)).toBe(
      `http://localhost:3000/shop/${id}`,
    );
  });

  it("SEO-32 unknown shop id falls back", async () => {
    const id = nextId();
    vi.mocked(getStorefront).mockResolvedValue(null);
    const md = await metaFor(id);
    expect(titleOf(md)).toMatch(
      new RegExp(`^Shop #${id.slice(0, 6)}( \\| Agora)?$`),
    );
    expect(imagesOf(md)).toEqual([]);
    expect(String(md.alternates?.canonical)).toBe(
      `http://localhost:3000/shop/${id}`,
    );
  });

  it("SEO-11 getStorefront is called once per request", async () => {
    const id = nextId();
    vi.mocked(getStorefront).mockResolvedValue({
      sellerId: id,
      displayName: "Cửa hàng Mai",
    } as never);
    await metaFor(id);
    await ShopPage({
      params: Promise.resolve({ id }),
      searchParams: Promise.resolve({}),
    });
    expect(getStorefront).toHaveBeenCalledTimes(1);
  });
});
