import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const APP_DIR = dirname(fileURLToPath(import.meta.url));
const SHOP = join(APP_DIR, "(shop)");
const ADMIN = join(SHOP, "admin");
const LOGIN = join(SHOP, "login", "page.tsx");

function pages(dir: string): string[] {
  const out: string[] = [];
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) out.push(...pages(full));
    else if (name === "page.tsx") out.push(full);
  }
  return out;
}

// Body of `export const metadata ... = { ... };` (first top-level block), or null.
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

function declaresNoReferrer(file: string): boolean {
  const block = metadataBlock(readFileSync(file, "utf8"));
  return block !== null && /\breferrer\s*:\s*["']no-referrer["']/.test(block);
}

describe("referrer metadata", () => {
  it("login and all admin pages set no-referrer (Referrer hygiene: every admin page and login declare no-referrer)", () => {
    const expected = [
      LOGIN,
      ...["cockpit", "users", "audit", "listings", "cases"].map((p) =>
        join(ADMIN, p, "page.tsx"),
      ),
    ];
    const missing = expected
      .filter((f) => !declaresNoReferrer(f))
      .map((f) => relative(APP_DIR, f));
    expect(missing).toEqual([]);
  });

  it("every admin page sets no-referrer (Referrer hygiene: a new admin page without the metadata fails the build)", () => {
    const found = pages(ADMIN);
    expect(found.length).toBeGreaterThanOrEqual(5);
    const missing = found
      .filter((f) => !declaresNoReferrer(f))
      .map((f) => relative(APP_DIR, f));
    expect(missing).toEqual([]);
  });

  it("admin audit page declares no-referrer (Referrer hygiene: admin audit page declares no-referrer)", () => {
    expect(declaresNoReferrer(join(ADMIN, "audit", "page.tsx"))).toBe(true);
  });

  it("scanner rejects a metadata block lacking referrer (Referrer hygiene: a new admin page without the metadata fails the build)", () => {
    expect(
      metadataBlock('export const metadata = { title: "x" };'),
    ).not.toMatch(/referrer/);
    expect(metadataBlock("export default function P() {}")).toBeNull();
  });
});
