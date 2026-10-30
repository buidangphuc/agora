import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export const root = join(dirname(fileURLToPath(import.meta.url)), "..");

const SKIP = new Set(["node_modules", ".next", "generated"]);
const SOURCE = /\.(?:ts|tsx|mjs|js|jsx|css)$/;

export function* walk(dir) {
  for (const name of readdirSync(dir)) {
    if (SKIP.has(name)) continue;
    const full = join(dir, name);
    if (statSync(full).isDirectory()) yield* walk(full);
    else if (SOURCE.test(name)) yield full;
  }
}

export function sourceFiles(dirs = ["packages", "templates"]) {
  return dirs.flatMap((d) => [...walk(join(root, d))]);
}

export function read(file) {
  return readFileSync(file, "utf8");
}

export function isMain(metaUrl) {
  return process.argv[1] && fileURLToPath(metaUrl) === process.argv[1];
}
