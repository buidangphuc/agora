#!/usr/bin/env node
/**
 * Token lint: component code must use design tokens (UI_SYSTEM_DESIGN.md section 2),
 * never raw colour or arbitrary values.
 *
 * Flags, in src/**\/*.tsx (excluding src/generated/**):
 *   - raw hex colours            #ee4d2d, #fff
 *   - rgb()/rgba()/hsl()/hsla()  literals
 *   - arbitrary Tailwind values  text-[9px], max-w-[1200px], bg-[#fff]
 *   - inline style colours       style={{ color: ... }}
 *
 * A line is exempt when it carries `tokens-allow: <reason>` (in a // or {/* *\/} comment).
 * Exit code is 1 when any violation is found; each is printed as `file:line  token`.
 */
import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const ALLOW = /tokens-allow:\s*\S/;
const HEX =
  /#(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{4}|[0-9a-fA-F]{3})\b/g;
const COLOR_FN = /\b(?:rgba?|hsla?)\(/g;
const ARBITRARY = /(?<![\w-])[\w:/.%-]*[\w]-\[[^\]\s]+\]/g;
const STYLE_BLOCK = /style=\{\{([\s\S]*?)\}\}/g;
const STYLE_COLOR_KEY =
  /\b(?:color|background|backgroundColor|borderColor|fill|stroke|outlineColor|boxShadow)\s*:/g;

function isCommentLine(line) {
  const t = line.trim();
  return t.startsWith("//") || t.startsWith("*") || t.startsWith("/*");
}

/**
 * Scan one source text. Returns [{ line, token }] sorted by line.
 * @param {string} text
 */
export function scanSource(text) {
  const lines = text.split("\n");
  const found = [];
  const push = (line, token) => found.push({ line, token });

  lines.forEach((line, i) => {
    if (ALLOW.test(line) || isCommentLine(line)) return;
    for (const re of [HEX, COLOR_FN, ARBITRARY]) {
      re.lastIndex = 0;
      for (const m of line.matchAll(re)) push(i + 1, m[0]);
    }
  });

  // Multi-line inline style objects: report the colour key on its own line.
  for (const block of text.matchAll(STYLE_BLOCK)) {
    const start = text.slice(0, block.index).split("\n").length;
    const body = block[1];
    STYLE_COLOR_KEY.lastIndex = 0;
    for (const key of body.matchAll(STYLE_COLOR_KEY)) {
      const line = start + body.slice(0, key.index).split("\n").length - 1;
      if (ALLOW.test(lines[line - 1] ?? "")) continue;
      // Values that are CSS variables / tokens are fine; a literal is not.
      const rest = body.slice(key.index + key[0].length).split(/[,}]/)[0];
      if (/var\(--color-/.test(rest)) continue;
      push(line, `style ${key[0].trim()}`);
    }
  }

  return found.sort((a, b) => a.line - b.line);
}

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) {
      if (name === "generated") continue;
      yield* walk(full);
    } else if (name.endsWith(".tsx")) {
      yield full;
    }
  }
}

/**
 * Scan a directory tree; returns [{ file, line, token }].
 * @param {string} srcDir
 * @param {string} [root]
 */
export function scanTree(srcDir, root = dirname(srcDir)) {
  const out = [];
  for (const file of walk(srcDir)) {
    for (const v of scanSource(readFileSync(file, "utf8"))) {
      out.push({ file: relative(root, file), ...v });
    }
  }
  return out;
}

const invokedDirectly =
  process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1];

if (invokedDirectly) {
  const root = join(dirname(fileURLToPath(import.meta.url)), "..");
  const only = process.argv.slice(2);
  let violations = scanTree(join(root, "src"), root);
  if (only.length > 0) {
    violations = violations.filter((v) =>
      only.some((p) => v.file.startsWith(p)),
    );
  }
  for (const v of violations) console.log(`${v.file}:${v.line}  ${v.token}`);
  console.log(`\ntoken lint: ${violations.length} violation(s)`);
  process.exit(violations.length > 0 ? 1 : 0);
}
