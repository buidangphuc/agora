import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { describe, expect, it } from "vitest";

const SRC = join(__dirname, "..");
const PROP =
  "(?:text|bg|border(?:-[trblxy])?|ring|divide|placeholder|from|via|to|shadow)";
const warmPalette = new Set(["features/listing/LiveFlashSaleStock.tsx"]);
const darkOrWarmBlocks = new Set([
  "features/listing/LiveFlashSaleStock.tsx",
  "features/student/StudentRentalView.tsx",
]);
const gradientLine = /(?<![\w-])bg-gradient-/;

const banned: {
  name: string;
  re: RegExp;
  allow?: Set<string>;
  skipGradients?: boolean;
}[] = [
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
  {
    name: "raw tint scale",
    re: new RegExp(
      `(?<![\\w-])${PROP}-(?:red|rose|emerald|green|teal|amber|yellow|slate|blue|sky|purple)-\\d`,
    ),
    allow: darkOrWarmBlocks,
    skipGradients: true,
  },
  {
    name: "white or black with an opacity modifier",
    re: new RegExp(`(?<![\\w-])${PROP}-(?:white|black)/\\d`),
    allow: darkOrWarmBlocks,
    skipGradients: true,
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

  it.each(banned)(
    "has no $name in app source",
    ({ re, allow, skipGradients }) => {
      const hits = files
        .filter((f) => !allow?.has(f.rel))
        .flatMap((f) =>
          f.text
            .split("\n")
            .map((line, i) => ({ at: `${f.rel}:${i + 1}`, line }))
            .filter(({ line }) => re.test(line))
            .filter(({ line }) => !(skipGradients && gradientLine.test(line)))
            .map(({ at }) => at),
        );
      expect(hits).toEqual([]);
    },
  );
});
