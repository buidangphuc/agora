// @vitest-environment node
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import postcss from "postcss";
import tailwindcss from "tailwindcss";
import { describe, expect, it } from "vitest";

import config from "../../tailwind.config";

const globalsCss = readFileSync(
  fileURLToPath(new URL("./globals.css", import.meta.url)),
  "utf8",
);

async function build(classes: string): Promise<string> {
  const result = await postcss([
    tailwindcss({
      ...config,
      content: [{ raw: `<div class="${classes}"></div>` }],
    }),
  ]).process(globalsCss, { from: undefined });
  return result.css;
}

/** Value of a declaration inside the first rule whose selector matches. */
function declaration(css: string, selector: string, prop: string) {
  const root = postcss.parse(css);
  let found: string | undefined;
  root.walkRules(selector, (rule) => {
    rule.walkDecls(prop, (decl) => {
      found ??= decl.value;
    });
  });
  return found;
}

const BRAND = "#ee4d2d";

describe("design tokens", () => {
  it("Tier 1: bg-primary-500 resolves to the brand colour", async () => {
    const css = await build("bg-primary-500");
    expect(declaration(css, ".bg-primary-500", "background-color")).toContain(
      "rgb(238 77 45",
    );
  });

  it("Tier 2: bg-action-primary reads --color-action-primary, which is the brand colour", async () => {
    const css = await build("bg-action-primary");
    expect(declaration(css, ".bg-action-primary", "background-color")).toBe(
      "var(--color-action-primary)",
    );
    expect(declaration(css, ":root", "--color-action-primary")).toBe(BRAND);
  });

  it("Tier 2: every semantic alias is defined on :root and exposed as a colour", async () => {
    const aliases = [
      "action-primary",
      "action-primary-hover",
      "surface-page",
      "surface-card",
      "surface-muted",
      "border-subtle",
      "border-strong",
      "text-primary",
      "text-secondary",
      "text-disabled",
      "text-inverse",
      "danger",
      "promo",
      "success",
      "focus-ring",
    ];
    const css = await build("");
    for (const name of aliases) {
      expect(
        declaration(css, ":root", `--color-${name}`),
        `--color-${name}`,
      ).toBeTruthy();
      const colors = config.theme?.extend?.colors as Record<string, unknown>;
      expect(colors[name], `colour ${name}`).toBe(`var(--color-${name})`);
    }
  });

  it("type scale: no generated font-size is below 12px or off the 12/14/16/20/24 scale", async () => {
    const css = await build(
      "text-xs text-sm text-base text-lg text-xl text-2xl text-3xl text-4xl text-5xl",
    );
    const sizes = new Set<number>();
    postcss.parse(css).walkRules(/^\.text-/, (rule) => {
      rule.walkDecls("font-size", (decl) => {
        sizes.add(Number.parseFloat(decl.value) * 16);
      });
    });
    expect(sizes.size).toBeGreaterThan(0);
    for (const px of sizes) {
      expect([12, 14, 16, 20, 24]).toContain(px);
    }
  });
});
