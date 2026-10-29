import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { isMain } from "./lib.mjs";

const CLASS_GROUPS = {
  radius: /\brounded(?:-[a-z0-9]+)?\b/g,
  spacing:
    /\b(?:-?m[trblxy]?|p[trblxy]?|gap(?:-[xy])?|space-[xy])-[0-9a-z]+\b/g,
  text: /\btext-(?:xs|sm|base|lg|xl|[2-9]xl)\b/g,
};

function* pageFiles(dir) {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) yield* pageFiles(full);
    else if (/\.(tsx|ts)$/.test(name) && !/\.test\./.test(name)) yield full;
  }
}

export function measure(text) {
  const buttons = [...text.matchAll(/<Button\b([^>]*)>/g)].map((m) => {
    const v = m[1].match(/variant=["']([a-z]+)["']/);
    return v ? v[1] : "primary";
  });
  const classes = {};
  for (const [group, re] of Object.entries(CLASS_GROUPS)) {
    classes[group] = [...new Set(text.match(re) ?? [])].sort();
  }
  return {
    buttons: {
      total: buttons.length,
      primary: buttons.filter((b) => b === "primary").length,
      variants: [...new Set(buttons)].sort(),
    },
    classes,
  };
}

export function measureDir(dir) {
  const out = {};
  for (const file of pageFiles(dir)) {
    out[relative(dir, file)] = measure(readFileSync(file, "utf8"));
  }
  return out;
}

if (isMain(import.meta.url)) {
  const dir = resolve(process.argv[2] ?? "templates/app/app/(public)/shop");
  const result = measureDir(dir);
  console.log(JSON.stringify(result, null, 2));
  const union = (group) =>
    [...new Set(Object.values(result).flatMap((r) => r.classes[group]))].sort();
  console.log("\nunion across screens:");
  for (const g of Object.keys(CLASS_GROUPS))
    console.log(`${g}: ${union(g).join(" ")}`);
  const multi = Object.entries(result).filter(([, r]) => r.buttons.primary > 1);
  console.log(`\nscreens with more than one primary Button: ${multi.length}`);
  process.exit(multi.length > 0 ? 1 : 0);
}
