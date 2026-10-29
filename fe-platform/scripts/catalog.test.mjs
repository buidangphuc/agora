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
      "Button",
      "Checkbox",
      "Dialog",
      "Input",
      "Link",
      "Select",
      "Table",
      "ToastProvider",
      "useToast",
    ]);
  });

  it("gives every entry a purpose", () => {
    for (const c of catalog.components)
      expect(c.purpose.length).toBeGreaterThan(5);
  });

  it("reads variants and sizes from the prop types", () => {
    const props = Object.fromEntries(
      byName.Button.props.map((p) => [p.name, p]),
    );
    expect(props.variant.type).toBe('"primary" | "secondary" | "ghost"');
    expect(props.size.type).toBe('"sm" | "md" | "lg"');
    expect(byName.Button.native).toBe("button");
  });

  it("marks client components and required props", () => {
    expect(byName.Button.client).toBe(false);
    expect(byName.Table.client).toBe(false);
    for (const n of ["Select", "Checkbox", "Dialog", "ToastProvider"]) {
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
