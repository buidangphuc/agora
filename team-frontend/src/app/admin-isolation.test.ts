// @vitest-environment node
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { describe, expect, it } from "vitest";

const SRC = join(__dirname, "..");
const APP = __dirname;
const ADMIN_LITERAL = /["'`]\/admin(?=[/"'`?#]|$)/;

function* files(dir: string): Generator<string> {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) {
      if (name === "generated") continue;
      yield* files(full);
    } else if (/\.(ts|tsx)$/.test(name) && !/\.test\.tsx?$/.test(name)) {
      yield full;
    }
  }
}

function* dirs(dir: string): Generator<string> {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) {
      yield full;
      yield* dirs(full);
    }
  }
}

describe("admin isolation", () => {
  it("has no admin route segment in the buyer app", () => {
    const routes = [...dirs(APP)]
      .map((d) => relative(APP, d))
      .filter((d) => d.split("/").includes("admin"));
    expect(routes).toEqual([]);
  });

  it("has no href or route string pointing at /admin", () => {
    const hits: string[] = [];
    for (const file of files(SRC)) {
      readFileSync(file, "utf8")
        .split("\n")
        .forEach((line, i) => {
          if (ADMIN_LITERAL.test(line))
            hits.push(`${relative(SRC, file)}:${i + 1}`);
        });
    }
    expect(hits).toEqual([]);
  });

  it("detects an admin link", () => {
    expect(ADMIN_LITERAL.test('<Link href="/admin/cockpit">')).toBe(true);
    expect(ADMIN_LITERAL.test("redirect(`/admin`)")).toBe(true);
    expect(ADMIN_LITERAL.test('href="/administrator"')).toBe(false);
    expect(ADMIN_LITERAL.test('fetch("/api/admin/metrics")')).toBe(false);
  });
});
