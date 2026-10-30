import { relative } from "node:path";
import { isMain, read, root, sourceFiles } from "./lib.mjs";

const MARKERS =
  /@ts-ignore|@ts-expect-error|eslint-disable|biome-ignore|tokens-allow/;

function violationFor(line) {
  if (/@ts-ignore/.test(line)) return "ts-ignore is forbidden";
  if (/@ts-expect-error(?!\s+\S)/.test(line))
    return "ts-expect-error needs a description";
  if (
    /eslint-disable/.test(line) &&
    !/eslint-disable\S*\s+\S+.*--\s*\S/.test(line)
  ) {
    return "eslint-disable needs a rule and a reason after --";
  }
  if (
    /biome-ignore/.test(line) &&
    !/biome-ignore\s+\S+(?:\s+\S+)?:\s*\S/.test(line)
  ) {
    return "biome-ignore needs a reason after the colon";
  }
  if (/tokens-allow(?!:\s*\S)/.test(line)) return "tokens-allow needs a reason";
  return null;
}

export function scanDisables(text) {
  const violations = [];
  let count = 0;
  text.split("\n").forEach((line, i) => {
    if (!MARKERS.test(line)) return;
    count += 1;
    const reason = violationFor(line);
    if (reason) violations.push({ line: i + 1, reason });
  });
  return { count, violations };
}

if (isMain(import.meta.url)) {
  let total = 0;
  const violations = [];
  for (const file of sourceFiles()) {
    const r = scanDisables(read(file));
    total += r.count;
    for (const v of r.violations)
      violations.push({ file: relative(root, file), ...v });
  }
  for (const v of violations) console.log(`${v.file}:${v.line}  ${v.reason}`);
  console.log(
    `\ndisable check: ${total} disable(s), ${violations.length} violation(s)`,
  );
  process.exit(violations.length > 0 ? 1 : 0);
}
