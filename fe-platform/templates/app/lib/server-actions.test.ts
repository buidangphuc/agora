import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const appRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

function* actionFiles(dir: string): Generator<string> {
  for (const name of readdirSync(dir)) {
    if (name === "node_modules" || name === ".next") continue;
    const full = join(dir, name);
    if (statSync(full).isDirectory()) yield* actionFiles(full);
    else if (/\.(ts|tsx)$/.test(name) && !/\.test\./.test(name)) {
      if (/^\s*["']use server["']/.test(readFileSync(full, "utf8"))) yield full;
    }
  }
}

describe("server action files", () => {
  const files = [...actionFiles(appRoot)];

  it("finds the action files", () => {
    expect(files.length).toBeGreaterThanOrEqual(5);
  });

  it("export only async functions, as Next requires", () => {
    const bad: string[] = [];
    for (const file of files) {
      for (const line of readFileSync(file, "utf8").split("\n")) {
        if (!/^export\s/.test(line)) continue;
        if (!/^export\s+async\s+function\s/.test(line))
          bad.push(`${relative(appRoot, file)}: ${line.trim()}`);
      }
    }
    expect(bad).toEqual([]);
  });
});
