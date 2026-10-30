import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { describe, expect, it } from "vitest";

const SRC = join(__dirname, "..");
const PROP =
  "(?:text|bg|border(?:-[trblxy])?|ring|divide|placeholder|from|via|to|shadow)";
const warmPalette = new Set([
  "features/listing/LiveFlashSaleStock.tsx",
  "features/admin/CockpitView.tsx",
]);

const banned: { name: string; re: RegExp; allow?: Set<string> }[] = [
  {
    name: "legacy brand colour",
    re: new RegExp(`(?<![\\w-])${PROP}-brand\\b`),
  },
  { name: "legacy shopee shadow or colour", re: /(?<![\w-])\w+-shopee\b/ },
  { name: "legacy mall colour", re: new RegExp(`(?<![\\w-])${PROP}-mall\\b`) },
  { name: "raw gray scale", re: new RegExp(`(?<![\\w-])${PROP}-gray-\\d`) },
  { name: "raw indigo scale", re: new RegExp(`(?<![\\w-])${PROP}-indigo-\\d`) },
  {
    name: "raw orange scale",
    re: new RegExp(`(?<![\\w-])${PROP}-orange-\\d`),
    allow: warmPalette,
  },
];

function* sources(dir: string): Generator<string> {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) {
      if (name !== "generated") yield* sources(full);
    } else if (/\.tsx?$/.test(name) && !/\.test\.tsx?$/.test(name)) {
      yield full;
    }
  }
}

describe("raw palette classes", () => {
  const files = [...sources(SRC)].map((path) => ({
    rel: relative(SRC, path),
    text: readFileSync(path, "utf8"),
  }));

  it.each(banned)("has no $name in app source", ({ re, allow }) => {
    const hits = files
      .filter((f) => !allow?.has(f.rel))
      .flatMap((f) =>
        f.text
          .split("\n")
          .map((line, i) => ({ at: `${f.rel}:${i + 1}`, line }))
          .filter(({ line }) => re.test(line))
          .map(({ at }) => at),
      );
    expect(hits).toEqual([]);
  });
});
