import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { components } from "@fe-platform/tokens";
import postcss from "postcss";
import tailwindcss from "tailwindcss";
import config from "../tailwind.config.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const appDir = resolve(here, "..", "app");

function* files(dir) {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) yield* files(full);
    else if (/\.(tsx|ts)$/.test(name)) yield full;
  }
}

function classesOf(node) {
  if (typeof node === "string") return node.split(/\s+/).filter(Boolean);
  return Object.values(node).flatMap(classesOf);
}

function escapeClass(name) {
  return name.replace(/[:/[\]().%]/g, (c) => `\\${c}`);
}

const used = new Set();
for (const file of files(appDir)) {
  const text = readFileSync(file, "utf8");
  for (const m of text.matchAll(/className=(?:"([^"]+)"|\{`([^`]+)`\})/g)) {
    for (const c of (m[1] ?? m[2]).split(/\s+/).filter(Boolean)) used.add(c);
  }
}
for (const c of classesOf(components)) used.add(c);

const content = config.content.map((g) => resolve(here, "..", g));
const result = await postcss([tailwindcss({ ...config, content })]).process(
  "@tailwind utilities;",
  { from: undefined },
);
const missing = [...used].filter(
  (c) => !result.css.includes(`.${escapeClass(c)}`),
);
for (const c of missing) console.log(`no CSS generated for class: ${c}`);
console.log(`\nclass check: ${missing.length} missing of ${used.size} classes`);
process.exit(missing.length > 0 ? 1 : 0);
