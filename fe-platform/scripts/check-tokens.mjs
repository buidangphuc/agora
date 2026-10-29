import { relative } from "node:path";
import { isMain, read, root, sourceFiles } from "./lib.mjs";

const RULES = [
  { rule: "raw-colour-literal", re: /#[0-9a-fA-F]{3,8}\b/ },
  { rule: "raw-colour-function", re: /\b(?:rgba?|hsla?|oklch|oklab)\(/ },
  { rule: "arbitrary-value", re: /\b[a-z][a-z0-9-]*-\[[^\]\s]+\]/ },
];

export function scanTokens(text) {
  const out = [];
  const lines = text.split("\n");
  lines.forEach((line, i) => {
    if (/tokens-allow:\s*\S/.test(line)) return;
    if (i > 0 && /tokens-allow:\s*\S/.test(lines[i - 1])) return;
    for (const { rule, re } of RULES) {
      if (re.test(line)) out.push({ line: i + 1, rule });
    }
  });
  return out;
}

export function scanTree(files) {
  const found = [];
  for (const file of files) {
    const path = file.replaceAll("\\", "/");
    if (/(^|\/)packages\/tokens\//.test(path)) continue;
    for (const v of scanTokens(read(file))) {
      found.push({ file: relative(root, file), ...v });
    }
  }
  return found;
}

if (isMain(import.meta.url)) {
  const violations = scanTree(sourceFiles());
  for (const v of violations) console.log(`${v.file}:${v.line}  ${v.rule}`);
  console.log(`\ntoken check: ${violations.length} violation(s)`);
  process.exit(violations.length > 0 ? 1 : 0);
}
