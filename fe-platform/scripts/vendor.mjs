import { spawnSync } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { isMain, root } from "./lib.mjs";

export const PACKAGES = [
  { dir: "packages/tokens", file: "fe-platform-tokens.tgz" },
  { dir: "packages/ui", file: "fe-platform-ui.tgz" },
  { dir: "packages/api-client", file: "fe-platform-api-client.tgz" },
];

export function targets() {
  const pkg = JSON.parse(readFileSync(join(root, "package.json"), "utf8"));
  return (pkg.vendor?.targets ?? []).map((t) => resolve(root, t));
}

export function pack() {
  const tmp = mkdtempSync(join(tmpdir(), "fe-vendor-"));
  const out = new Map();
  try {
    for (const p of PACKAGES) {
      const run = spawnSync(
        "npm",
        [
          "pack",
          `./${p.dir}`,
          "--pack-destination",
          tmp,
          "--json",
          "--ignore-scripts",
        ],
        { cwd: root, encoding: "utf8" },
      );
      if (run.status !== 0)
        throw new Error(`npm pack ${p.dir} failed: ${run.stderr}`);
      const filename = JSON.parse(run.stdout)[0].filename;
      out.set(p.file, readFileSync(join(tmp, filename)));
    }
  } finally {
    rmSync(tmp, { recursive: true, force: true });
  }
  return out;
}

export function diffVendored(packed, dir) {
  const problems = [];
  for (const [file, bytes] of packed) {
    const target = join(dir, file);
    if (!existsSync(target)) problems.push(`${target}: missing`);
    else if (!readFileSync(target).equals(bytes))
      problems.push(`${target}: differs`);
  }
  return problems;
}

export function write(packed, dir) {
  mkdirSync(dir, { recursive: true });
  for (const [file, bytes] of packed) writeFileSync(join(dir, file), bytes);
}

if (isMain(import.meta.url)) {
  const check = process.argv.includes("--check");
  const dirs = targets();
  if (dirs.length === 0) {
    console.error("vendor: no vendor.targets in package.json");
    process.exit(1);
  }
  const packed = pack();
  if (check) {
    const problems = dirs.flatMap((d) => diffVendored(packed, d));
    for (const p of problems) console.error(`vendor: ${p}`);
    console.log(`vendor check: ${problems.length} problem(s)`);
    process.exit(problems.length === 0 ? 0 : 1);
  }
  for (const d of dirs) {
    write(packed, d);
    console.log(`vendor: wrote ${[...packed.keys()].join(", ")} to ${d}`);
  }
}
