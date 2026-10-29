import { describe, expect, it } from "vitest";
import { scanImports } from "./check-boundaries.mjs";
import { scanDisables } from "./check-disables.mjs";
import { scanTokens } from "./check-tokens.mjs";

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

  it("flags an app importing generated code or the transport directly", () => {
    const text = [
      'import a from "@connectrpc/connect";',
      'import b from "@fe-platform/api-client/src/generated/x";',
      'import c from "@bufbuild/protobuf";',
    ].join("\n");
    const hits = scanImports("templates/app/app/x.ts", text);
    expect(hits.map((h) => h.rule)).toEqual([
      "app-imports-transport",
      "app-imports-transport",
      "app-imports-transport",
    ]);
    expect(scanImports("packages/api-client/src/client.ts", text)).toEqual([]);
  });

  it("flags a client component importing server-only code", () => {
    const text = [
      '"use client";',
      'import { createGatewayClients } from "@fe-platform/api-client";',
      'import { getToken } from "../lib/session";',
    ].join("\n");
    const hits = scanImports("templates/app/app/x.tsx", text);
    expect(hits.map((h) => h.rule)).toEqual([
      "client-imports-server-only",
      "client-imports-server-only",
    ]);
    const server = text.replace('"use client";\n', "");
    expect(scanImports("templates/app/app/x.tsx", server)).toEqual([]);
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

describe("scanTokens", () => {
  it("flags raw colours and arbitrary values", () => {
    const text = [
      'const a = "#fff";',
      'const b = "w-[13px]";',
      'const c = "rgb(0 0 0)";',
    ].join("\n");
    expect(scanTokens(text).map((v) => v.rule)).toEqual([
      "raw-colour-literal",
      "arbitrary-value",
      "raw-colour-function",
    ]);
  });

  it("allows a literal that states a reason", () => {
    const text = [
      "// tokens-allow: brand asset colour from vendor spec",
      'const a = "#fff";',
      'const b = "bg-[#000]"; // tokens-allow: third-party embed',
    ].join("\n");
    expect(scanTokens(text)).toEqual([]);
  });

  it("passes token-backed utilities", () => {
    expect(scanTokens('const a = "bg-surface-card p-4 rounded-lg";')).toEqual(
      [],
    );
  });
});
