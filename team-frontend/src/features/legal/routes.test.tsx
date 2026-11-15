import { readFileSync } from "node:fs";
import { join } from "node:path";

import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { LEGAL_SLUGS, readLegalPage } from "./content";

vi.mock("next/navigation", () => ({
  notFound: () => {
    throw new Error("NEXT_NOT_FOUND");
  },
}));

const gateway = vi.hoisted(() => ({
  listCategories: vi.fn(async () => []),
  readSitemapListings: vi.fn(async () => []),
}));
vi.mock("@/lib/gateway/listings", () => ({
  listCategories: gateway.listCategories,
}));
vi.mock("@/lib/gateway/sitemap", () => ({
  readSitemapListings: gateway.readSitemapListings,
}));

const PAGE = "@/app/(shop)/(legal)/[slug]/page";
const SRC = join(__dirname, "../..");

beforeEach(() => {
  gateway.listCategories.mockClear();
  gateway.readSitemapListings.mockClear();
});

describe("legal route", () => {
  it("[All seven slugs resolve] generateStaticParams lists the seven slugs and dynamicParams is false", async () => {
    const mod = await import(/* @vite-ignore */ PAGE);
    expect(mod.dynamicParams).toBe(false);
    const params = await mod.generateStaticParams();
    expect(params.map((p: { slug: string }) => p.slug).sort()).toEqual(
      [...LEGAL_SLUGS].sort(),
    );
  });

  it("[A legal page renders title, version and placeholder] renders h1, Phiên bản and placeholder", async () => {
    const mod = await import(/* @vite-ignore */ PAGE);
    const page = readLegalPage("dieu-khoan-su-dung");
    render(
      await mod.default({
        params: Promise.resolve({ slug: "dieu-khoan-su-dung" }),
      }),
    );
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe(
      page?.title,
    );
    expect(document.body.textContent).toMatch(
      new RegExp(`Phiên bản\\s*:?\\s*${page?.version}`),
    );
    expect(document.body.textContent).toContain(
      "NỘI DUNG DO BỘ PHẬN PHÁP CHẾ CUNG CẤP",
    );
  });

  it("[An unknown legal slug is not found] page calls notFound", async () => {
    const mod = await import(/* @vite-ignore */ PAGE);
    await expect(
      mod.default({
        params: Promise.resolve({ slug: "chinh-sach-khong-ton-tai" }),
      }),
    ).rejects.toThrow("NEXT_NOT_FOUND");
  });

  it("[Legal pages need no login] the legal route group makes no gateway or session call", () => {
    const src = readFileSync(
      join(SRC, "app/(shop)/(legal)/[slug]/page.tsx"),
      "utf8",
    );
    expect(src).not.toMatch(
      /lib\/gateway|lib\/session|requireSession|redirect\(/,
    );
  });
});

describe("sitemap and robots", () => {
  it("[All seven slugs resolve] sitemap contains exactly the 7 legal URLs", async () => {
    const { default: sitemap } = await import("@/app/sitemap");
    const urls = (await sitemap()).map((e) => e.url);
    const legal = urls.filter((u) =>
      [...LEGAL_SLUGS].some((s) => u.endsWith(`/${s}`)),
    );
    expect(legal).toHaveLength(7);
    expect(new Set(legal).size).toBe(7);
  });

  it("[Legal pages need no login] robots does not disallow any legal slug", async () => {
    const { default: robots } = await import("@/app/robots");
    const rules = robots().rules;
    const disallow = ([] as string[]).concat(
      ...(Array.isArray(rules) ? rules : [rules]).map((r) => r.disallow ?? []),
    );
    for (const s of LEGAL_SLUGS) {
      expect(disallow.some((d) => `/${s}`.startsWith(d))).toBe(false);
    }
  });
});
