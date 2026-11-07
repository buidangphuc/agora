import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const APP_DIR = dirname(fileURLToPath(import.meta.url));
const SHOP = join(APP_DIR, "(shop)");

const PRIVATE_LAYOUTS: { segment: string; file: string }[] = [
  ...[
    "account",
    "cart",
    "chat",
    "notifications",
    "favorites",
    "dev",
    "login",
    "register",
  ].map((s) => ({ segment: s, file: join(SHOP, s, "layout.tsx") })),
  { segment: "(checkout)", file: join(APP_DIR, "(checkout)", "layout.tsx") },
];

function metadataBlock(src: string): string | null {
  const m = /export\s+const\s+metadata\b[^=]*=\s*\{/.exec(src);
  if (!m) return null;
  let depth = 1;
  let i = m.index + m[0].length;
  const start = i;
  while (i < src.length && depth > 0) {
    if (src[i] === "{") depth++;
    else if (src[i] === "}") depth--;
    i++;
  }
  return src.slice(start, i - 1);
}

function robotsBlock(block: string): string | null {
  const m = /\brobots\s*:\s*\{/.exec(block);
  if (!m) return null;
  let depth = 1;
  let i = m.index + m[0].length;
  const start = i;
  while (i < block.length && depth > 0) {
    if (block[i] === "{") depth++;
    else if (block[i] === "}") depth--;
    i++;
  }
  return block.slice(start, i - 1);
}

function exportsNoindex(src: string): boolean {
  const block = metadataBlock(src);
  if (block === null) return false;
  const robots = robotsBlock(block);
  return (
    robots !== null &&
    /\bindex\s*:\s*false\b/.test(robots) &&
    /\bfollow\s*:\s*false\b/.test(robots)
  );
}

function violations(): string[] {
  return PRIVATE_LAYOUTS.filter(
    ({ file }) =>
      !existsSync(file) || !exportsNoindex(readFileSync(file, "utf8")),
  ).map(({ segment }) => segment);
}

describe("private segment layouts", () => {
  it("SEO-18 SEO-19 SEO-20 every private segment layout exports noindex", () => {
    expect(violations()).toEqual([]);
  });

  it("SEO-20 scanner rejects a private layout without robots", () => {
    expect(exportsNoindex('export const metadata = { title: "x" };')).toBe(
      false,
    );
    expect(exportsNoindex("export default function L() {}")).toBe(false);
    expect(
      exportsNoindex(
        "export const metadata = { robots: { index: true, follow: false } };",
      ),
    ).toBe(false);
    expect(
      exportsNoindex(
        "export const metadata = { robots: { index: false, follow: true } };",
      ),
    ).toBe(false);
    expect(
      exportsNoindex(
        "export const metadata = { robots: { index: false, follow: false } };",
      ),
    ).toBe(true);
  });

  it("SEO-19 login layout noindex beside page referrer", () => {
    const layout = join(SHOP, "login", "layout.tsx");
    const page = join(SHOP, "login", "page.tsx");
    expect(existsSync(layout)).toBe(true);
    expect(exportsNoindex(readFileSync(layout, "utf8"))).toBe(true);
    const pageBlock = metadataBlock(readFileSync(page, "utf8"));
    expect(pageBlock).not.toBeNull();
    expect(pageBlock).toMatch(/\breferrer\s*:\s*["']no-referrer["']/);
  });

  it("SEO-18 private layouts carry no canonical or Open Graph keys", () => {
    for (const { file } of PRIVATE_LAYOUTS) {
      if (!existsSync(file)) continue;
      const block = metadataBlock(readFileSync(file, "utf8")) ?? "";
      expect(block).not.toMatch(/\bopenGraph\b|\balternates\b/);
    }
  });
});
