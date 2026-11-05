#!/usr/bin/env node
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const compiled = join(root, "node_modules/next/dist/compiled");
const MARKER =
  "A previously unvisited boundary must have exactly one root segment";

const candidates = [
  ...readdirSync(join(compiled, "next-server"))
    .filter((f) => /^app-page.*\.runtime\.(prod|dev)\.js$/.test(f))
    .map((f) => join(compiled, "next-server", f)),
  ...["react-dom", "react-dom-experimental"].flatMap((pkg) =>
    readdirSync(join(compiled, pkg, "cjs"))
      .filter((f) => /^react-dom-server.*\.js$/.test(f))
      .map((f) => join(compiled, pkg, "cjs", f)),
  ),
];

const FORMS = [
  {
    buggy:
      /(var (\w+)=(\w+)\.boundary;if\(null===\2\)return \w+\(\w+,\w+,\3,\w+\);if\()(\2\.parentFlushed=!0,)/g,
    fix: (_m, head, _b, seg, tail) => `${head}${seg}.boundary=null,${tail}`,
    fixed:
      /var (\w+)=(\w+)\.boundary;if\(null===\1\)return \w+\(\w+,\w+,\2,\w+\);if\(\2\.boundary=null,\1\.parentFlushed=!0,/,
  },
  {
    buggy:
      /(boundary = segment\.boundary;\s*if \(null === boundary\)\s*return flushSubtree\(request, destination, segment, hoistableState\);)(\s*)(boundary\.parentFlushed = !0;)/g,
    fix: (_m, head, ws, tail) =>
      `${head}${ws}segment.boundary = null;${ws}${tail}`,
    fixed:
      /return flushSubtree\(request, destination, segment, hoistableState\);\s*segment\.boundary = null;\s*boundary\.parentFlushed = !0;/,
  },
];

let failed = false;
for (const file of candidates) {
  let src = readFileSync(file, "utf8");
  if (!src.includes(MARKER)) continue;
  let state = "unknown";
  for (const form of FORMS) {
    if (form.buggy.test(src)) {
      form.buggy.lastIndex = 0;
      src = src.replace(form.buggy, form.fix);
      writeFileSync(file, src);
      state = "patched";
      break;
    }
    if (form.fixed.test(src)) {
      state = "already patched";
      break;
    }
  }
  console.log(
    `patch-react-dom-boundary: ${state} ${file.slice(root.length + 1)}`,
  );
  if (state === "unknown") failed = true;
}
if (failed) {
  console.error(
    "patch-react-dom-boundary: flushSegment changed; re-check whether the vendored react-dom still needs this patch.",
  );
  process.exit(1);
}
