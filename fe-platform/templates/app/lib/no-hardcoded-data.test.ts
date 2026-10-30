import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const appRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

function* sources(dir: string): Generator<string> {
  for (const name of readdirSync(dir)) {
    if (name === "node_modules" || name === ".next") continue;
    const full = join(dir, name);
    if (statSync(full).isDirectory()) yield* sources(full);
    else if (/\.(ts|tsx)$/.test(name) && !/\.test\.(ts|tsx)$/.test(name))
      yield full;
  }
}

const FORBIDDEN: [string, RegExp][] = [
  ["a static product list", /SHOP_PRODUCTS/],
  ["a price field literal", /\b(priceMinor|unitPrice|totalAmount)\s*:\s*\d/],
  ["the removed demo modules", /from\s+["'][^"']*lib\/shop(-delivery)?["']/],
  ["local cart or delivery state", /DELIVERY_COOKIE|parseCart|serializeCart/],
];

describe("no hardcoded data in the app", () => {
  it("keeps invented products, prices and cart state out of source files", () => {
    const hits: string[] = [];
    for (const file of sources(appRoot)) {
      const text = readFileSync(file, "utf8");
      for (const [what, re] of FORBIDDEN)
        if (re.test(text)) hits.push(`${relative(appRoot, file)}: ${what}`);
    }
    expect(hits).toEqual([]);
  });

  it("scans real files", () => {
    expect([...sources(appRoot)].length).toBeGreaterThan(10);
  });
});
