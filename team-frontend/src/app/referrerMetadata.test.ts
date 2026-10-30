import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const APP_DIR = dirname(fileURLToPath(import.meta.url));
const SHOP = join(APP_DIR, "(shop)");
const LOGIN = join(SHOP, "login", "page.tsx");

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
  it("login sets no-referrer (Referrer hygiene: login declares no-referrer)", () => {
    expect(declaresNoReferrer(LOGIN)).toBe(true);
  });

  it("scanner rejects a metadata block lacking referrer (Referrer hygiene: a new admin page without the metadata fails the build)", () => {
    expect(
      metadataBlock('export const metadata = { title: "x" };'),
    ).not.toMatch(/referrer/);
    expect(metadataBlock("export default function P() {}")).toBeNull();
  });
});
