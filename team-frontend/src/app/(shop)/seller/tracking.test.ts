import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, resolve } from "node:path";
import { describe, expect, it } from "vitest";

const SRC = resolve(__dirname, "../../..");
const ROOTS = ["app/(shop)/seller", "app/(shop)/sell", "features/seller"];
const FORBIDDEN =
  /TrackLink|TrackImpression|SearchImpressions|AnalyticsProvider|data-track|data-placement|data-impression/;

function* files(dir: string): Generator<string> {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) yield* files(full);
    else if (/\.(tsx?|ts)$/.test(name) && !/\.test\./.test(name)) yield full;
  }
}

describe("seller routes carry no tracking hooks", () => {
  it("never import or render the tracking components or data attributes", () => {
    const offenders: string[] = [];
    for (const root of ROOTS) {
      for (const file of files(join(SRC, root))) {
        if (FORBIDDEN.test(readFileSync(file, "utf8"))) offenders.push(file);
      }
    }
    expect(offenders).toEqual([]);
  });

  it("the Workplace product links stay plain /listing/[id] links", () => {
    const table = readFileSync(
      join(SRC, "features/seller/ProductTable.tsx"),
      "utf8",
    );
    expect(table).toContain("href={`/listing/${l.id}`}");
    expect(table).not.toMatch(/TrackLink/);
  });
});
