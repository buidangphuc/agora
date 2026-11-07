import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const APP_DIR = dirname(fileURLToPath(import.meta.url));
const LAYOUT = join(APP_DIR, "layout.tsx");
const SUFFIXES = ["| Sàn Thương Mại Điện Tử", "| Marketplace"];

function pages(dir: string): string[] {
  const out: string[] = [];
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) out.push(...pages(full));
    else if (name === "page.tsx") out.push(full);
  }
  return out;
}

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

function rootBlock(): string {
  return metadataBlock(readFileSync(LAYOUT, "utf8")) ?? "";
}

describe("title suffix (admin)", () => {
  it("no hand-written suffix in page titles (SEO-43)", () => {
    const offenders = pages(APP_DIR)
      .filter((file) => {
        const block = metadataBlock(readFileSync(file, "utf8"));
        return block !== null && SUFFIXES.some((s) => block.includes(s));
      })
      .map((file) => relative(APP_DIR, file));
    expect(offenders).toEqual([]);
  });

  it("scanner flags a suffixed title (SEO-43)", () => {
    const block = metadataBlock('export const metadata = { title: "X | Sàn Thương Mại Điện Tử" };');
    expect(SUFFIXES.some((s) => block!.includes(s))).toBe(true);
  });

  it("root layout denies indexing (SEO-43)", () => {
    expect(rootBlock()).toMatch(
      /robots\s*:\s*\{(?=[^}]*\bindex\s*:\s*false)(?=[^}]*\bfollow\s*:\s*false)[^}]*\}/,
    );
  });

  it("root layout title is default plus %s | Agora template (SEO-43)", () => {
    const block = rootBlock();
    expect(block).toMatch(/default\s*:\s*["']Quản trị sàn \| Agora["']/);
    expect(block).toMatch(/template\s*:\s*["']%s \| Agora["']/);
  });

  it("root layout keeps no-referrer (SEO-43)", () => {
    expect(rootBlock()).toMatch(/\breferrer\s*:\s*["']no-referrer["']/);
  });
});
