import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const SRC_DIR = dirname(fileURLToPath(import.meta.url));

function sourceFiles(dir: string): string[] {
  const out: string[] = [];
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) out.push(...sourceFiles(full));
    else if (/\.(ts|tsx)$/.test(name)) out.push(full);
  }
  return out;
}

const WRAPPED = /\(\s*(["'])use (?:client|server)\1\s*\)/;
const DIRECTIVE_LINE = /^(["'])use (?:client|server)\1;?\s*$/;

// Returns human-readable problems for one file's source text.
function directiveProblems(source: string): string[] {
  const problems: string[] = [];
  const noBlockComments = source.replace(/\/\*[\s\S]*?\*\//g, (m) =>
    m.replace(/[^\n]/g, " "),
  );
  const lines = noBlockComments.split("\n").map((l) => l.replace(/^\s*\/\/.*$/, ""));

  lines.forEach((line, i) => {
    if (WRAPPED.test(line))
      problems.push(`line ${i + 1}: directive wrapped in parentheses`);
  });

  // Module-level (column 0) directive must be the first statement.
  const idx = lines.findIndex((l) => DIRECTIVE_LINE.test(l));
  if (idx > 0 && lines.slice(0, idx).some((l) => l.trim() !== ""))
    problems.push(`line ${idx + 1}: statement precedes directive`);
  return problems;
}

describe("use client / use server directives", () => {
  it("[Directives stay real directives] no source file has a wrapped or misplaced directive", () => {
    const offenders: string[] = [];
    for (const file of sourceFiles(SRC_DIR)) {
      if (/\.test\.(ts|tsx)$/.test(file)) continue;
      const text = readFileSync(file, "utf8");
      if (!/use (client|server)/.test(text)) continue;
      for (const p of directiveProblems(text))
        offenders.push(`${file.slice(SRC_DIR.length)}: ${p}`);
    }
    expect(offenders).toEqual([]);
  });

  it("[Directives stay real directives] accepts a leading directive, with comments and blank lines before it", () => {
    expect(directiveProblems('"use client";\nimport a from "a";\n')).toEqual([]);
    expect(directiveProblems("'use server'\nimport a from 'a';\n")).toEqual([]);
    expect(
      directiveProblems('// license\n/* block\n x */\n\n"use client";\nimport a from "a";\n'),
    ).toEqual([]);
  });

  it("[Directives stay real directives] flags a parenthesised directive", () => {
    expect(directiveProblems('("use client");\nimport a from "a";\n')).toHaveLength(1);
  });

  it("[Directives stay real directives] flags an import above the directive", () => {
    expect(
      directiveProblems('import a from "a";\n"use client";\n'),
    ).toHaveLength(1);
  });

  it("[Directives stay real directives] ignores indented function-level directives and files without any", () => {
    expect(
      directiveProblems('export async function f() {\n  "use server";\n}\n'),
    ).toEqual([]);
    expect(directiveProblems("export const x = 1;\n")).toEqual([]);
  });
});
