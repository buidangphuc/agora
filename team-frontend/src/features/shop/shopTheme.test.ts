import { primitives } from "@fe-platform/tokens";
import { describe, expect, it } from "vitest";
import { SHOP_THEMES, resolveShopTheme } from "./shopTheme";

const palette = primitives.color as Record<string, string>;

function colorOf(cls: string): string {
  const token = cls.split(/\s+/).find((c) => /^(bg|text)-/.test(c));
  if (!token) throw new Error(`no colour class in "${cls}"`);
  const hex = palette[token.replace(/^(bg|text)-/, "")];
  if (!hex) throw new Error(`class "${token}" is not a palette token`);
  return hex;
}

function luminance(hex: string): number {
  const h = hex.replace("#", "").slice(0, 6);
  const [r, g, b] = [0, 2, 4].map((i) => {
    const c = Number.parseInt(h.slice(i, i + 2), 16) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

describe("shopTheme", () => {
  it("closed table holds exactly default, dark, festive (scenario: A themed shop renders its theme)", () => {
    expect(Object.keys(SHOP_THEMES).sort()).toEqual([
      "dark",
      "default",
      "festive",
    ]);
    expect(resolveShopTheme("festive").key).toBe("festive");
    expect(resolveShopTheme("dark").key).toBe("dark");
  });

  it("unknown key resolves to default (scenario: An unknown theme key falls back to default)", () => {
    for (const bad of [
      "",
      "neon",
      "Dark",
      "__proto__",
      "constructor",
      undefined,
      null,
    ]) {
      const t = resolveShopTheme(bad as string);
      expect(t.key).toBe("default");
      expect(t).toEqual(resolveShopTheme("default"));
    }
  });

  it("never echoes a raw key into class strings (scenario: An unknown theme key falls back to default)", () => {
    const t = resolveShopTheme("<script>x</script>");
    expect(`${t.headerBg} ${t.headerText} ${t.accent}`).not.toContain("script");
  });

  it("themes differ in header colour (scenario: A themed shop renders its theme)", () => {
    const bgs = Object.keys(SHOP_THEMES).map(
      (k) => resolveShopTheme(k).headerBg,
    );
    expect(new Set(bgs).size).toBe(3);
  });

  it("every theme meets AA contrast (scenario: Theme text stays readable)", () => {
    for (const key of Object.keys(SHOP_THEMES)) {
      const t = resolveShopTheme(key);
      const ratio = contrast(colorOf(t.headerBg), colorOf(t.headerText));
      expect(ratio, key).toBeGreaterThanOrEqual(4.5);
    }
  });
});
