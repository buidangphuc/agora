// @vitest-environment node
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { primitives, resolveShadows, resolveTheme } from "@fe-platform/tokens";
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

const palette = primitives.color as Record<string, string>;
const BRAND = resolveTheme("light")["action-primary"];

function rgbOf(hex: string): string {
  const n = Number.parseInt(hex.slice(1), 16);
  return `rgb(${(n >> 16) & 255} ${(n >> 8) & 255} ${n & 255}`;
}

describe("design tokens", () => {
  it("Tier 1: bg-primary-500 resolves to the platform primary scale", async () => {
    const css = await build("bg-primary-500");
    expect(declaration(css, ".bg-primary-500", "background-color")).toContain(
      rgbOf(palette["primary-500"]),
    );
  });

  it("Tier 2: bg-action-primary reads --color-action-primary, which is the brand colour", async () => {
    const css = await build("bg-action-primary");
    expect(declaration(css, ".bg-action-primary", "background-color")).toBe(
      "var(--color-action-primary)",
    );
    expect(declaration(css, ":root", "--color-action-primary")).toBe(BRAND);
    expect(BRAND).toBe(palette["primary-600"]);
  });

  it("Tier 2: every platform semantic name is defined on :root and exposed as a colour", async () => {
    const css = await build("");
    const colors = config.theme?.extend?.colors as Record<string, unknown>;
    for (const [name, value] of Object.entries(resolveTheme("light"))) {
      expect(declaration(css, ":root", `--color-${name}`), name).toBe(value);
      expect(colors[name], `colour ${name}`).toBe(`var(--color-${name})`);
    }
  });

  it("emits the light theme only: no dark or data-theme block", async () => {
    const css = await build("");
    expect(css).not.toContain("prefers-color-scheme");
    expect(css).not.toContain("data-theme");
  });

  it("exposes the platform shadows", async () => {
    const css = await build("shadow-card shadow-pop");
    expect(declaration(css, ":root", "--shadow-card")).toBe(
      resolveShadows("light").card,
    );
    expect(css).toContain(".shadow-card");
    expect(css).toContain(".shadow-pop");
  });

  it("gray is the neutral scale so gray-* and neutral-* agree", async () => {
    const css = await build("bg-gray-100 bg-neutral-100");
    const gray = declaration(css, ".bg-gray-100", "background-color");
    const neutral = declaration(css, ".bg-neutral-100", "background-color");
    expect(gray).toBe(neutral);
    expect(gray).toContain(rgbOf(palette["neutral-100"]));
  });

  it("legacy colour groups no longer carry the old orange, red, yellow or green", () => {
    const old = [
      "#ee4d2d",
      "#d73211",
      "#ffeee8",
      "#f05d40",
      "#d0011b",
      "#b00016",
      "#ffbe00",
      "#00bfa5",
    ];
    const colors = config.theme?.extend?.colors as Record<
      string,
      Record<string, string>
    >;
    const legacy = JSON.stringify([
      colors.brand,
      colors.mall,
      colors.shopee,
    ]).toLowerCase();
    for (const hex of old) expect(legacy, hex).not.toContain(hex);
  });

  it("platform utilities for touch height and safe areas exist", async () => {
    const css = await build("min-h-touch pt-safe pb-safe");
    expect(declaration(css, ".min-h-touch", "min-height")).toBe("44px");
    expect(declaration(css, ".pt-safe", "padding-top")).toContain(
      "safe-area-inset-top",
    );
    expect(declaration(css, ".pb-safe", "padding-bottom")).toContain(
      "safe-area-inset-bottom",
    );
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

  it("type scale: no generated font-size is below 12px or off the 12/14/16/20/24/30/36 scale", async () => {
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
      expect([12, 14, 16, 20, 24, 30, 36]).toContain(px);
    }
  });
});
