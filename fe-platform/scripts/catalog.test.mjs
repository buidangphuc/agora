import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { buildCatalog, collectComponents, render } from "./catalog.mjs";

describe("catalog", () => {
  const catalog = buildCatalog();
  const byName = Object.fromEntries(catalog.components.map((c) => [c.name, c]));

  it("lists every exported ui component and hook", () => {
    expect(Object.keys(byName).sort()).toEqual([
      "Badge",
      "Button",
      "Card",
      "CardContent",
      "CardDescription",
      "CardFooter",
      "CardHeader",
      "CardTitle",
      "Checkbox",
      "ClosableTag",
      "Dialog",
      "Empty",
      "Input",
      "Link",
      "Result",
      "Select",
      "Table",
      "Tag",
      "ToastProvider",
      "useToast",
    ]);
  });

  it("tags every entry with the platforms it runs on", () => {
    for (const c of catalog.components)
      expect(c.platforms).toEqual(["web", "wap"]);
  });

  it("gives every entry a purpose", () => {
    for (const c of catalog.components)
      expect(c.purpose.length).toBeGreaterThan(5);
  });

  it("reads variants and sizes from the prop types", () => {
    const props = Object.fromEntries(
      byName.Button.props.map((p) => [p.name, p]),
    );
    const members = (type) => type.split(" | ").sort();
    expect(members(props.variant.type)).toEqual([
      '"danger"',
      '"ghost"',
      '"primary"',
      '"secondary"',
    ]);
    expect(members(props.size.type)).toEqual(['"lg"', '"md"', '"sm"', '"xs"']);
    expect(byName.Button.native).toBe("button");
  });

  it("marks client components and required props", () => {
    expect(byName.Button.client).toBe(false);
    expect(byName.Table.client).toBe(false);
    for (const n of ["Tag", "Badge", "Empty", "Result"])
      expect(byName[n].client).toBe(false);
    for (const n of [
      "Select",
      "Checkbox",
      "ClosableTag",
      "Dialog",
      "ToastProvider",
    ]) {
      expect(byName[n].client).toBe(true);
    }
    const label = byName.Input.props.find((p) => p.name === "label");
    expect(label.required).toBe(true);
  });

  it("lists token names from the tokens package", () => {
    expect(catalog.tokens.colors).toContain("action-primary");
    expect(catalog.tokens.radius.length).toBeGreaterThan(0);
  });

  it("is byte-identical across runs", () => {
    expect(render(buildCatalog())).toBe(render(buildCatalog()));
  });

  it("has no deprecated entries today", () => {
    expect(catalog.components.filter((c) => "deprecated" in c)).toEqual([]);
  });

  it("copies a @deprecated tag into the entry", () => {
    const dir = mkdtempSync(join(tmpdir(), "catalog-"));
    try {
      const entry = join(dir, "index.ts");
      writeFileSync(
        entry,
        "/**\n * Old thing.\n * @deprecated Use B; removed in 2.0.0.\n */\nexport function A(p: { x: string }) { return p.x; }\n",
      );
      const options = {
        target: ts.ScriptTarget.ES2022,
        module: ts.ModuleKind.ESNext,
        moduleResolution: ts.ModuleResolutionKind.Bundler,
        strict: true,
        noEmit: true,
        skipLibCheck: true,
      };
      const [a] = collectComponents(entry, options);
      expect(a.purpose).toBe("Old thing.");
      expect(a.deprecated).toBe("Use B; removed in 2.0.0.");
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  it("fails generation when an export has no JSDoc purpose", () => {
    const dir = mkdtempSync(join(tmpdir(), "catalog-"));
    try {
      const entry = join(dir, "index.ts");
      writeFileSync(
        entry,
        "export function A(p: { x: string }) { return p.x; }\n",
      );
      const options = {
        target: ts.ScriptTarget.ES2022,
        module: ts.ModuleKind.ESNext,
        moduleResolution: ts.ModuleResolutionKind.Bundler,
        strict: true,
        noEmit: true,
        skipLibCheck: true,
      };
      expect(() => collectComponents(entry, options)).toThrow(
        /no JSDoc purpose/,
      );
      writeFileSync(
        entry,
        "/** Says x. */\nexport function A(p: { x: string }) { return p.x; }\n",
      );
      expect(collectComponents(entry, options)[0].purpose).toBe("Says x.");
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
