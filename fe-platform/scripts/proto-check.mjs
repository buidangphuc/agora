import { spawnSync } from "node:child_process";
import {
  existsSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join, relative } from "node:path";
import { isMain, root } from "./lib.mjs";

const pkg = join(root, "packages", "api-client");
const committed = join(pkg, "src", "generated");
const protoDir = join(root, "..", "platform-core", "packages", "proto");

function tree(dir) {
  const out = new Map();
  const walk = (d) => {
    for (const e of readdirSync(d, { withFileTypes: true })) {
      const full = join(d, e.name);
      if (e.isDirectory()) walk(full);
      else out.set(relative(dir, full), readFileSync(full, "utf8"));
    }
  };
  if (existsSync(dir)) walk(dir);
  return out;
}

export function diffTrees(a, b) {
  const names = new Set([...a.keys(), ...b.keys()]);
  const diffs = [];
  for (const name of [...names].sort()) {
    if (!a.has(name)) diffs.push(`${name}: missing from committed output`);
    else if (!b.has(name)) diffs.push(`${name}: no longer generated`);
    else if (a.get(name) !== b.get(name)) diffs.push(`${name}: differs`);
  }
  return diffs;
}

export function protoCheck() {
  if (!existsSync(protoDir)) {
    return [`proto source not found at ${protoDir}`];
  }
  const tmp = mkdtempSync(join(tmpdir(), "fe-proto-"));
  try {
    const run = spawnSync("npx", ["buf", "generate", "--output", tmp], {
      cwd: pkg,
      encoding: "utf8",
    });
    if (run.status !== 0) return [`buf generate failed: ${run.stderr.trim()}`];
    return diffTrees(tree(committed), tree(join(tmp, "src", "generated")));
  } finally {
    rmSync(tmp, { recursive: true, force: true });
  }
}

if (isMain(import.meta.url)) {
  const problems = protoCheck();
  for (const p of problems) console.log(p);
  console.log(`\nproto check: ${problems.length} problem(s)`);
  if (problems.length > 0) {
    console.log(
      "fix: npm run proto -w packages/api-client, then commit src/generated",
    );
  }
  process.exit(problems.length > 0 ? 1 : 0);
}
