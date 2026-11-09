import { readFileSync, readdirSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const APP_DIR = dirname(fileURLToPath(import.meta.url));
const SRC = join(APP_DIR, "..");

function sources(dir: string): string[] {
  const out: string[] = [];
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    const p = join(dir, e.name);
    if (e.isDirectory()) {
      if (e.name === "generated" || e.name === "node_modules") continue;
      out.push(...sources(p));
    } else if (
      /\.(ts|tsx)$/.test(e.name) &&
      !/\.(test|spec)\.(ts|tsx)$/.test(e.name)
    ) {
      out.push(p);
    }
  }
  return out;
}

function offenders(files: string[], patterns: RegExp[]): string[] {
  const hits: string[] = [];
  for (const f of files) {
    const src = readFileSync(f, "utf8");
    for (const re of patterns) {
      if (re.test(src)) hits.push(`${relative(SRC, f)}: ${re.source}`);
    }
  }
  return hits;
}

describe("demo brand strings", () => {
  it("SEO-28 no demo brand string in source", () => {
    expect(
      offenders(sources(SRC), [
        /Marketplace Showcase/,
        /Polyrepo/,
        /BATDONGSAN SINH VIÊN/,
      ]),
    ).toEqual([]);
  });
});

describe("hand-written title suffixes", () => {
  it("SEO-1 no page under src/app writes a legacy title suffix", () => {
    expect(
      offenders(sources(APP_DIR), [
        /\|\s*Sàn Thương Mại Điện Tử/,
        /\|\s*Kênh người bán/,
        /\|\s*Marketplace/,
      ]),
    ).toEqual([]);
  });
});
