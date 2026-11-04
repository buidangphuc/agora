import { readdirSync, readFileSync, statSync } from "node:fs";
import { basename, join, resolve } from "node:path";

import { describe, expect, it } from "vitest";

const REMOVED = ["Cần tài khoản", "Người Bán"].join(" ");

const appRoot = process.cwd();
const repoRoot = resolve(appRoot, "..");

const SKIP_DIRS = new Set(["node_modules", ".next", ".git", "coverage"]);
const ABSENCE = [
  /queryByText/,
  /queryAllByText/,
  /not\.toBeInTheDocument/,
  /toBeNull\(\)/,
  /to_have_count\(0\)/,
  /\bnot_to\b/,
  /\bno\s+[`"\\]/,
  /\bnot show\b/,
  /\bnor\b/,
  /\bneither\b/,
  /\bshows no\b/,
  /\bsearched\b/,
];

function walk(dir: string, out: string[]): string[] {
  for (const name of readdirSync(dir)) {
    if (SKIP_DIRS.has(name)) continue;
    const full = join(dir, name);
    const st = statSync(full);
    if (st.isDirectory()) walk(full, out);
    else out.push(full);
  }
  return out;
}

describe("removed seller notice text", () => {
  it("[Seller onboarding UI: no source file shows the removed text] no source shows the removed seller notice text", () => {
    const files = [
      ...walk(appRoot, []),
      ...walk(join(repoRoot, "platform-e2e/tests/e2e/step_definitions"), []),
      ...walk(join(repoRoot, "platform-e2e/tests/e2e/features"), []),
    ].filter((f) => basename(f) !== "removedText.test.ts");

    expect(files.length).toBeGreaterThan(50);

    const offenders: string[] = [];
    for (const file of files) {
      if (!/\.(tsx?|jsx?|mjs|cjs|json|ya?ml|md|py|feature|css)$/.test(file)) {
        continue;
      }
      const lines = readFileSync(file, "utf8").split("\n");
      lines.forEach((line, i) => {
        if (!line.includes(REMOVED)) return;
        if (ABSENCE.some((re) => re.test(line))) return;
        offenders.push(`${file.slice(repoRoot.length + 1)}:${i + 1}`);
      });
    }
    expect(offenders).toEqual([]);
  });

  it("[Seller onboarding UI: no source file shows the removed text] the scan sees the e2e step definitions and the seller app", () => {
    const steps = walk(
      join(repoRoot, "platform-e2e/tests/e2e/step_definitions"),
      [],
    );
    expect(steps.some((f) => f.endsWith("seller_cockpit_steps.py"))).toBe(true);
    expect(walk(appRoot, []).some((f) => f.endsWith("FEATURES.yaml"))).toBe(
      true,
    );
  });
});
