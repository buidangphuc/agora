import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getListing } from "@/lib/gateway/listings";

vi.mock("next/navigation", () => ({
  notFound: () => {
    throw new Error("NEXT_NOT_FOUND");
  },
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => "/listing/L",
  useSearchParams: () => new URLSearchParams(),
}));
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
vi.mock("@/lib/gateway/listings", () => ({
  getListing: vi.fn(),
  getCategory: vi.fn(),
  getStorefront: vi.fn(),
}));
vi.mock("@/lib/gateway/engagement", () => ({
  isFavorite: vi.fn(),
  listCollections: vi.fn(),
  recordView: vi.fn(),
}));
vi.mock("@/lib/gateway/notification", () => ({
  AlertType: {},
  listAlertSubscriptions: vi.fn(),
}));
vi.mock("@/lib/gateway/promotion", () => ({ getActiveFlashSale: vi.fn() }));
vi.mock("@/lib/gateway/reviews", () => ({
  getListingRatingSummary: vi.fn(),
  getShopRatingSummary: vi.fn(),
  listReviews: vi.fn(),
  listReviewsPage: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn(() => null) }));
vi.mock("@/features/cart/actions", () => ({ addToCartAction: vi.fn() }));
vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));

import ListingLayout from "./layout";
import { generateMetadata } from "./page";
import { makeListing } from "./pdpFixtures";

let seq = 0;
function nextId() {
  seq += 1;
  return `meta-${seq}`;
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

function descriptionOf(md: { description?: unknown }) {
  return md.description as string | null | undefined;
}

async function metaFor(id: string) {
  return generateMetadata({ params: Promise.resolve({ id }) });
}

beforeEach(() => {
  vi.clearAllMocks();
  Reflect.deleteProperty(process.env, "NEXT_PUBLIC_SITE_URL");
});

describe("listing generateMetadata", () => {
  it("SEO-4 builds title, canonical and Open Graph product tags", async () => {
    const id = nextId();
    vi.mocked(getListing).mockResolvedValue(
      makeListing({
        id,
        title: "Điện thoại Demo",
        description: "Mô tả ngắn",
        imageUrl: "http://localhost:9000/listing-images/a.jpg",
      }),
    );
    const md = await metaFor(id);
    expect(titleOf(md)).toMatch(/^Điện thoại Demo( \| Agora)?$/);
    expect(String(md.alternates?.canonical)).toBe(
      `http://localhost:3000/listing/${id}`,
    );
    const og = md.openGraph as unknown as Record<string, unknown>;
    expect(og.type).toBe("product");
    expect(og.siteName).toBe("Agora");
    expect(String(og.url)).toBe(`http://localhost:3000/listing/${id}`);
    expect(String(og.title)).toMatch(/^Điện thoại Demo( \| Agora)?$/);
    expect(og.description).toBe("Mô tả ngắn");
    const images = ([] as unknown[]).concat(og.images ?? []);
    expect(
      images.map((i) =>
        typeof i === "string"
          ? i
          : String(i instanceof URL ? i : (i as { url: unknown }).url),
      ),
    ).toEqual(["http://localhost:9000/listing-images/a.jpg"]);
  });

  it("SEO-6 cuts a 400 character multi-line description at 160 with no line break", async () => {
    const id = nextId();
    const description = Array.from({ length: 20 }, (_, i) => `dong ${i} xx`)
      .join("\n")
      .padEnd(400, "y");
    vi.mocked(getListing).mockResolvedValue(makeListing({ id, description }));
    const md = await metaFor(id);
    const d = descriptionOf(md) ?? "";
    expect(d.length).toBeGreaterThan(0);
    expect(d.length).toBeLessThanOrEqual(160);
    expect(d).not.toMatch(/[\r\n]/);
    const od = (md.openGraph as { description?: string }).description ?? "";
    expect(od.length).toBeLessThanOrEqual(160);
    expect(od).not.toMatch(/[\r\n]/);
  });

  it("SEO-6 an empty description falls back to the Mua ... trên Agora sentence", async () => {
    const id = nextId();
    vi.mocked(getListing).mockResolvedValue(
      makeListing({ id, title: "Áo thun", description: "", price: 120000 }),
    );
    const md = await metaFor(id);
    const d = descriptionOf(md) ?? "";
    expect(d.startsWith("Mua Áo thun giá ")).toBe(true);
    expect(d.endsWith(" ₫ trên Agora.")).toBe(true);
    expect(d).toMatch(/120[.,\s ]?000/);
  });

  it("SEO-7 omits og:image when the listing has no image and keeps the other tags", async () => {
    const id = nextId();
    vi.mocked(getListing).mockResolvedValue(
      makeListing({ id, imageUrl: undefined, imageKeys: [] }),
    );
    const md = await metaFor(id);
    const og = md.openGraph as unknown as Record<string, unknown>;
    expect(og.images ?? []).toEqual([]);
    expect(og.type).toBe("product");
    expect(og.siteName).toBe("Agora");
    expect(og.title).toBeTruthy();
  });

  it("SEO-31 listing outage returns default metadata", async () => {
    const id = nextId();
    vi.mocked(getListing).mockRejectedValue(
      new ConnectError("down", Code.Unavailable),
    );
    const md = await metaFor(id);
    expect(md.openGraph).toBeUndefined();
    expect(md.alternates).toBeUndefined();
    const t = titleOf(md);
    expect(t === undefined || t === "Agora | Sàn thương mại điện tử").toBe(
      true,
    );
  });

  it("SEO-5 a missing listing carries no Open Graph tag and no canonical", async () => {
    const id = nextId();
    vi.mocked(getListing).mockResolvedValue(null);
    const md = await metaFor(id);
    expect(md.openGraph).toBeUndefined();
    expect(md.alternates).toBeUndefined();
  });

  it("SEO-8 getListing is called once per request", async () => {
    const id = nextId();
    vi.mocked(getListing).mockResolvedValue(makeListing({ id }));
    await ListingLayout({
      children: null,
      params: Promise.resolve({ id }),
    });
    await metaFor(id);
    expect(getListing).toHaveBeenCalledTimes(1);
  });
});
