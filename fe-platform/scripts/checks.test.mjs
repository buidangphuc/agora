import { describe, expect, it } from "vitest";
import { scanImports } from "./check-boundaries.mjs";
import { scanDisables } from "./check-disables.mjs";

describe("scanImports", () => {
  it("flags a public route importing internal code", () => {
    const hits = scanImports(
      "templates/app/app/(public)/page.tsx",
      'import { x } from "../(internal)/dashboard/x";',
    );
    expect(hits.map((h) => h.rule)).toEqual(["public-imports-internal"]);
  });

  it("flags a package importing from an app", () => {
    const hits = scanImports(
      "packages/ui/src/a.ts",
      'import a from "@fe-platform/app/x";',
    );
    expect(hits.map((h) => h.rule)).toEqual(["package-imports-app"]);
  });

  it("allows ordinary imports", () => {
    expect(
      scanImports(
        "templates/app/app/(public)/page.tsx",
        'import { y } from "react";',
      ),
    ).toEqual([]);
  });
});

describe("scanDisables", () => {
  it("rejects ts-ignore and reasonless disables", () => {
    const text = [
      "// @ts-ignore",
      "// biome-ignore lint/a11y/useButtonType",
      "// eslint-disable-next-line",
    ].join("\n");
    const r = scanDisables(text);
    expect(r.count).toBe(3);
    expect(r.violations).toHaveLength(3);
  });

  it("accepts disables that state a reason", () => {
    const text = [
      "// biome-ignore lint/a11y/useButtonType: submit handled by parent form",
      "// @ts-expect-error upstream type is wrong",
      "// eslint-disable-next-line no-console -- debug hook",
    ].join("\n");
    const r = scanDisables(text);
    expect(r.count).toBe(3);
    expect(r.violations).toEqual([]);
  });

  it("counts nothing in clean code", () => {
    expect(scanDisables("const a = 1;")).toEqual({ count: 0, violations: [] });
  });
});
