import { beforeEach, describe, expect, it, vi } from "vitest";

import { SortBy } from "@/generated/platform/search/v1/search_pb.js";
import { makeClients } from "@/lib/gateway/client";
import { getListing, listCategories } from "@/lib/gateway/listings";

vi.mock("@/lib/gateway/client", () => ({ makeClients: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({
  getToken: vi.fn(() => undefined),
  getPrincipal: vi.fn(() => null),
  SESSION_COOKIE: "session",
}));
vi.mock("@/lib/gateway/listings", () => ({
  getListing: vi.fn(),
  listCategories: vi.fn(),
}));

import sitemap, { revalidate } from "./sitemap";

const ORIGIN = "http://localhost:3000";
const rpc = vi.fn();

interface Hit {
  listingId: string;
  sellerId: string;
}

function hitsOf(total: number, sellerOf: (i: number) => string): Hit[] {
  return Array.from({ length: total }, (_, i) => ({
    listingId: `l${i}`,
    sellerId: sellerOf(i),
  }));
}

function serve(all: Hit[]) {
  rpc.mockImplementation(
    async (req: { page?: { cursor?: string; pageSize?: number } }) => {
      const size = req.page?.pageSize ?? 0;
      const start = Number(req.page?.cursor || "0");
      const end = Math.min(start + size, all.length);
      return {
        hits: all.slice(start, end),
        page: {
          nextCursor: end < all.length ? String(end) : "",
          total: BigInt(all.length),
        },
      };
    },
  );
}

function urls(entries: { url: string }[]) {
  return entries.map((e) => e.url);
}

beforeEach(() => {
  vi.clearAllMocks();
  Reflect.deleteProperty(process.env, "NEXT_PUBLIC_SITE_URL");
  vi.mocked(makeClients).mockReturnValue({
    search: { searchListings: rpc },
  } as never);
  vi.mocked(listCategories).mockResolvedValue([
    { id: "c1", name: "A" },
    { id: "c2", name: "B" },
    { id: "c3", name: "C" },
  ] as never);
});

describe("sitemap", () => {
  it("SEO-36 revalidates hourly", () => {
    expect(revalidate).toBe(3600);
  });

  it("SEO-23 lists home, categories, listings newest first and each shop once, all absolute", async () => {
    serve([
      { listingId: "l1", sellerId: "s1" },
      { listingId: "l2", sellerId: "s2" },
      { listingId: "l3", sellerId: "s1" },
      { listingId: "l4", sellerId: "s2" },
    ]);
    const got = urls(await sitemap());
    expect(got).toContain(`${ORIGIN}/`);
    for (const c of ["c1", "c2", "c3"])
      expect(got).toContain(`${ORIGIN}/search?category=${c}`);
    expect(got.filter((u) => u.includes("/listing/"))).toEqual([
      `${ORIGIN}/listing/l1`,
      `${ORIGIN}/listing/l2`,
      `${ORIGIN}/listing/l3`,
      `${ORIGIN}/listing/l4`,
    ]);
    expect(got.filter((u) => u.includes("/shop/")).sort()).toEqual([
      `${ORIGIN}/shop/s1`,
      `${ORIGIN}/shop/s2`,
    ]);
    for (const u of got) expect(u.startsWith(`${ORIGIN}/`)).toBe(true);
    expect(new Set(got).size).toBe(got.length);
  });

  it("SEO-24 asks the search index for published listings only, newest first, 50 per page", async () => {
    serve(hitsOf(3, () => "s1"));
    await sitemap();
    const req = rpc.mock.calls[0]?.[0];
    expect(req.filters?.status).toBe("published");
    expect(req.sortBy).toBe(SortBy.NEWEST);
    expect(req.page.pageSize).toBe(50);
  });

  it("SEO-24 never lists a private prefix", async () => {
    serve(hitsOf(3, () => "s1"));
    const got = urls(await sitemap());
    for (const p of [
      "/account",
      "/cart",
      "/checkout",
      "/seller",
      "/admin",
      "/login",
      "/register",
      "/chat",
      "/notifications",
      "/favorites",
      "/dev",
      "/api",
    ])
      expect(got.some((u) => u.startsWith(`${ORIGIN}${p}`))).toBe(false);
  });

  it("SEO-25 caps at 5000 newest listings", async () => {
    serve(hitsOf(5200, (i) => `s${i % 7}`));
    const got = urls(await sitemap());
    const listings = got.filter((u) => u.includes("/listing/"));
    expect(listings).toHaveLength(5000);
    expect(listings[0]).toBe(`${ORIGIN}/listing/l0`);
    expect(listings[4999]).toBe(`${ORIGIN}/listing/l4999`);
    expect(got).not.toContain(`${ORIGIN}/listing/l5000`);
    expect(got).not.toContain(`${ORIGIN}/listing/l5199`);
  });

  it("SEO-36 reads 5000 listings in 100 requests of page_size 50 with no getListing call", async () => {
    serve(hitsOf(5200, (i) => `s${i % 7}`));
    await sitemap();
    expect(rpc).toHaveBeenCalledTimes(100);
    for (const call of rpc.mock.calls) expect(call[0].page.pageSize).toBe(50);
    expect(getListing).not.toHaveBeenCalled();
  });

  it("SEO-45 empty seller_id adds no shop url", async () => {
    serve([
      { listingId: "a", sellerId: "" },
      { listingId: "b", sellerId: "s1" },
    ]);
    const got = urls(await sitemap());
    expect(got).toContain(`${ORIGIN}/listing/a`);
    expect(got).toContain(`${ORIGIN}/listing/b`);
    expect(got.filter((u) => u.includes("/shop/"))).toEqual([
      `${ORIGIN}/shop/s1`,
    ]);
  });

  it("SEO-26 gateway failure returns home", async () => {
    rpc.mockRejectedValue(new Error("unavailable"));
    vi.mocked(listCategories).mockRejectedValue(new Error("unavailable"));
    const got = urls(await sitemap());
    expect(got).toContain(`${ORIGIN}/`);
  });

  it("SEO-26 a failure after some pages keeps the entries already read", async () => {
    const all = hitsOf(120, (i) => `s${i % 3}`);
    serve(all);
    const ok = rpc.getMockImplementation();
    let n = 0;
    rpc.mockImplementation(async (req: unknown) => {
      n += 1;
      if (n === 2) throw new Error("boom");
      return ok?.(req);
    });
    const got = urls(await sitemap());
    expect(got).toContain(`${ORIGIN}/`);
    expect(got).toContain(`${ORIGIN}/listing/l0`);
    expect(got).toContain(`${ORIGIN}/listing/l49`);
    expect(got).not.toContain(`${ORIGIN}/listing/l50`);
  });
});
