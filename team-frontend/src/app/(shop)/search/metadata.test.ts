import { beforeEach, describe, expect, it, vi } from "vitest";

const loadCategories = vi.hoisted(() => vi.fn());
const listCategories = vi.hoisted(() => vi.fn());

vi.mock("@/features/search/data", () => ({
  loadSearch: vi.fn(),
  loadCategories,
  searchKey: (s: unknown) => JSON.stringify(s),
}));
vi.mock("@/lib/gateway/listings", () => ({ listCategories }));
vi.mock("@/features/search/SearchBlocks", () => ({
  ResultCount: () => null,
  FilterPanel: () => null,
  SearchResultsBlock: () => null,
}));
vi.mock("@/features/search/SavedSearches", () => ({
  SavedSearches: () => null,
}));
vi.mock("@/lib/gateway/search", () => ({
  listSavedSearches: async () => [],
}));

import { generateMetadata } from "./page";

type Raw = Record<string, string | string[] | undefined>;

function metaFor(searchParams: Raw) {
  return generateMetadata({ searchParams: Promise.resolve(searchParams) });
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

function robotsOf(md: { robots?: unknown }) {
  return md.robots as { index?: boolean; follow?: boolean } | undefined;
}

const PLAIN_TITLE = /^Tất cả sản phẩm( \| Agora)?$/;

beforeEach(() => {
  vi.clearAllMocks();
  Reflect.deleteProperty(process.env, "NEXT_PUBLIC_SITE_URL");
  loadCategories.mockResolvedValue([
    {
      id: "c1",
      name: "Điện thoại",
      slug: "",
      parentId: "",
      displayOrder: 0,
      iconUrl: "",
    },
  ]);
});

describe("search generateMetadata", () => {
  it("SEO-12 a category page is indexable with a clean canonical", async () => {
    const md = await metaFor({
      category: "c1",
      sort: "price_asc",
      page: "2",
    });
    expect(titleOf(md)).toMatch(/^Điện thoại( \| Agora)?$/);
    expect(String(md.alternates?.canonical)).toBe(
      "http://localhost:3000/search?category=c1",
    );
    expect(robotsOf(md)?.index).not.toBe(false);
    const og = md.openGraph as unknown as Record<string, unknown>;
    expect(og.type).toBe("website");
    expect(og.siteName).toBe("Agora");
    expect(md.description).toBe(
      "Mua Điện thoại chính hãng, giá tốt trên Agora.",
    );
  });

  it("SEO-35 plain search is indexable with its own title and description", async () => {
    const md = await metaFor({});
    expect(titleOf(md)).toMatch(PLAIN_TITLE);
    expect(md.description).toBe(
      "Tìm và mua sản phẩm từ nhiều người bán trên Agora.",
    );
    expect(String(md.alternates?.canonical)).toBe(
      "http://localhost:3000/search",
    );
    expect(robotsOf(md)?.index).not.toBe(false);
  });

  it("SEO-13 a free-text search is noindex,follow with no canonical and no query text", async () => {
    const q = "<script>x</script>";
    const md = await metaFor({ q });
    expect(robotsOf(md)).toMatchObject({ index: false, follow: true });
    expect(md.alternates?.canonical).toBeUndefined();
    expect(JSON.stringify(md)).not.toContain("script");
  });

  it("SEO-33 whitespace q is empty", async () => {
    const md = await metaFor({ q: "   " });
    expect(titleOf(md)).toMatch(PLAIN_TITLE);
    expect(String(md.alternates?.canonical)).toBe(
      "http://localhost:3000/search",
    );
    expect(robotsOf(md)?.index).not.toBe(false);
  });

  it("SEO-34 q wins over category", async () => {
    const md = await metaFor({ category: "c1", q: "ao" });
    expect(robotsOf(md)).toMatchObject({ index: false, follow: true });
    expect(md.alternates?.canonical).toBeUndefined();
    expect(titleOf(md)).not.toMatch(/Điện thoại/);
    expect(JSON.stringify(md)).not.toContain("ao");
  });

  it("SEO-14 unknown category uses plain search metadata", async () => {
    const md = await metaFor({ category: "no-such-category" });
    expect(titleOf(md)).toMatch(PLAIN_TITLE);
    expect(String(md.alternates?.canonical)).toBe(
      "http://localhost:3000/search",
    );
    expect(robotsOf(md)?.index).not.toBe(false);
  });

  it("SEO-15 categories failure falls back", async () => {
    loadCategories.mockRejectedValue(new Error("down"));
    const md = await metaFor({ category: "c1" });
    expect(titleOf(md)).toMatch(PLAIN_TITLE);
  });

  it("SEO-15 reads categories through the request-cached loader, not the gateway directly", async () => {
    await metaFor({ category: "c1" });
    expect(loadCategories).toHaveBeenCalledTimes(1);
    expect(listCategories).not.toHaveBeenCalled();
  });
});
