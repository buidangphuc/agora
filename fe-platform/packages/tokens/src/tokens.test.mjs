import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, readdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { checkPair, contrastRatio } from "./contrast.mjs";
import { pairs } from "./pairs.mjs";
import preset from "./preset.mjs";
import {
  THEMES,
  px,
  resolveShadows,
  resolveTheme,
  semanticNames,
  shadowNames,
  toCss,
  toJson,
} from "./resolve.mjs";
import { components, primitives, semantic } from "./tokens.mjs";

const here = dirname(fileURLToPath(import.meta.url));

describe("contrastRatio", () => {
  it("pins known ratios", () => {
    expect(contrastRatio("#000000", "#ffffff")).toBeCloseTo(21, 5);
    expect(contrastRatio("#ee4d2d", "#ffffff")).toBeCloseTo(3.66, 2);
    expect(contrastRatio("#d73211", "#ffffff")).toBeGreaterThan(4.5);
  });
});

describe("semantic tokens", () => {
  it("defines the same names in both themes", () => {
    expect(Object.keys(semantic.dark)).toEqual(Object.keys(semantic.light));
  });

  it("resolves every semantic value to a primitive", () => {
    for (const theme of THEMES) {
      const values = resolveTheme(theme);
      expect(Object.keys(values)).toEqual(semanticNames());
      for (const value of Object.values(values)) {
        expect(Object.values(primitives.color)).toContain(value);
      }
    }
  });
});

describe("contrast gate", () => {
  for (const theme of THEMES) {
    it(`every declared pair passes in ${theme}`, () => {
      const values = resolveTheme(theme);
      const failing = pairs
        .map((p) => checkPair(p, values))
        .filter((r) => !r.pass)
        .map((r) => `${r.fg}/${r.bg} ${r.ratio.toFixed(2)} < ${r.min}`);
      expect(failing).toEqual([]);
    });
  }

  it("covers action-primary with inverse text", () => {
    expect(
      pairs.some((p) => p.fg === "text-inverse" && p.bg === "action-primary"),
    ).toBe(true);
  });

  it("reports a deliberately low-contrast pair as failing", () => {
    const values = resolveTheme("light");
    const bad = checkPair(
      { fg: "text-disabled", bg: "surface-card", size: "normal" },
      values,
    );
    expect(bad.pass).toBe(false);
  });

  it("applies the large-text threshold", () => {
    const values = { a: "#ee4d2d", b: "#ffffff" };
    expect(checkPair({ fg: "a", bg: "b", size: "normal" }, values).pass).toBe(
      false,
    );
    expect(checkPair({ fg: "a", bg: "b", size: "large" }, values).pass).toBe(
      true,
    );
  });
});

describe("generated css", () => {
  it("matches the committed dist", () => {
    const committed = readFileSync(
      join(here, "..", "dist", "tokens.css"),
      "utf8",
    );
    expect(committed).toBe(toCss());
  });

  it("matches the committed type declarations", () => {
    const pkg = join(here, "..");
    const tmp = mkdtempSync(join(tmpdir(), "tokens-types-"));
    try {
      const run = spawnSync(
        "npx",
        ["tsc", "-p", "tsconfig.build.json", "--outDir", tmp],
        { cwd: pkg, encoding: "utf8" },
      );
      expect(run.status).toBe(0);
      const committedDir = join(pkg, "dist", "types");
      const names = readdirSync(tmp).sort();
      expect(readdirSync(committedDir).sort()).toEqual(names);
      for (const n of names) {
        expect(readFileSync(join(committedDir, n), "utf8")).toBe(
          readFileSync(join(tmp, n), "utf8"),
        );
      }
    } finally {
      rmSync(tmp, { recursive: true, force: true });
    }
  }, 30000);

  it("declares every semantic token in the dark block", () => {
    const css = toCss();
    for (const name of semanticNames()) {
      expect(css.split(`--color-${name}:`).length - 1).toBe(3);
    }
  });
});

describe("tailwind preset", () => {
  it("exposes semantic colours only, never primitives", () => {
    const keys = Object.keys(preset.theme.colors);
    expect(keys).toEqual(["transparent", "current", ...semanticNames()]);
    expect(keys.some((k) => k.startsWith("primary-"))).toBe(false);
  });

  it("replaces spacing, radius and font size with token values in px", () => {
    expect(preset.theme.spacing[4]).toBe("16px");
    expect(preset.theme.spacing[0]).toBe("0px");
    expect(preset.theme.borderRadius["2xl"]).toBe("16px");
    expect(preset.theme.fontSize.base).toEqual(["16px", "24px"]);
    expect(Object.keys(preset.theme.spacing)).toEqual(
      Object.keys(primitives.space),
    );
  });

  it("exposes breakpoints, touch height, shadows and safe-area utilities", () => {
    expect(preset.theme.screens).toEqual({
      sm: "640px",
      md: "768px",
      lg: "1024px",
    });
    expect(preset.theme.minHeight.touch).toBe("44px");
    expect(Object.keys(preset.theme.boxShadow)).toEqual([
      "none",
      ...shadowNames(),
    ]);
    const added = {};
    for (const plugin of preset.plugins) {
      plugin({ addUtilities: (u) => Object.assign(added, u) });
    }
    expect(Object.keys(added)).toEqual([".pt-safe", ".pb-safe"]);
  });
});

function classesOf(node) {
  if (typeof node === "string") return node.split(/\s+/);
  return Object.values(node).flatMap(classesOf);
}

describe("component class maps", () => {
  const names = new Set([...semanticNames(), "transparent", "current"]);
  const sides = new Set(["t", "r", "b", "l", "x", "y"]);
  const keywords = new Set(["left", "center", "right", "collapse"]);
  const space = new Set(Object.keys(primitives.space));
  const radius = new Set(Object.keys(primitives.radius));
  const sizes = new Set(Object.keys(primitives.fontSize));

  for (const [component, map] of Object.entries(components)) {
    it(`${component} uses only token-backed utilities`, () => {
      const bad = classesOf(map)
        .map((c) => c.split(":").pop())
        .filter((c) => {
          const m =
            /^(bg|text|border|ring|p|px|py|gap|rounded)(?:-(.+))?$/.exec(c);
          if (!m) return false;
          const [, kind, rest] = m;
          if (kind === "rounded")
            return rest !== undefined && !radius.has(rest);
          if (kind === "p" || kind === "px" || kind === "py" || kind === "gap")
            return !space.has(rest);
          if (rest === undefined) return false;
          if (sides.has(rest) || keywords.has(rest)) return false;
          if (kind === "text" && sizes.has(rest)) return false;
          if ((kind === "border" || kind === "ring") && /^\d+$/.test(rest))
            return false;
          const side = /^([trblxy])-(.+)$/.exec(rest);
          if (kind === "border" && side && sides.has(side[1]))
            return !names.has(side[2]);
          return !names.has(rest);
        });
      expect(bad).toEqual([]);
    });
  }
});

describe("platform-neutral token data", () => {
  const unit = /\d\s*(px|rem|em|pt|dp|sp)\b/;

  function strings(node) {
    if (typeof node === "string") return [node];
    if (node && typeof node === "object")
      return Object.values(node).flatMap(strings);
    return [];
  }

  it("holds raw numbers, never platform unit strings", () => {
    const scales = {
      space: primitives.space,
      radius: primitives.radius,
      fontSize: primitives.fontSize,
      breakpoint: primitives.breakpoint,
      touchTarget: primitives.touchTarget,
    };
    expect(strings(scales).filter((s) => unit.test(s))).toEqual([]);
    for (const n of [
      ...Object.values(primitives.space),
      ...Object.values(primitives.radius),
      ...Object.values(primitives.breakpoint),
      ...primitives.fontSize.base,
    ]) {
      expect(typeof n).toBe("number");
    }
    expect(typeof primitives.touchTarget).toBe("number");
  });

  it("matches the committed tokens.json", () => {
    const committed = readFileSync(
      join(here, "..", "dist", "tokens.json"),
      "utf8",
    );
    expect(committed).toBe(toJson());
  });

  it("agrees with tokens.css on every colour and shadow", () => {
    const json = JSON.parse(toJson());
    const css = readFileSync(join(here, "..", "dist", "tokens.css"), "utf8");
    const blocks = {
      light: css.split("\n\n")[0],
      dark: css.split("\n\n")[1],
    };
    for (const theme of THEMES) {
      for (const [name, value] of Object.entries(json.color[theme])) {
        expect(blocks[theme]).toContain(`--color-${name}: ${value};`);
      }
      for (const [name, value] of Object.entries(resolveShadows(theme))) {
        expect(blocks[theme]).toContain(`--shadow-${name}: ${value};`);
      }
      for (const name of shadowNames()) {
        expect(json.shadow[theme][name].length).toBeGreaterThan(0);
        for (const l of json.shadow[theme][name]) {
          expect(Object.keys(l)).toEqual(["x", "y", "blur", "spread", "color"]);
        }
      }
    }
  });

  it("formats zero without a bare unit", () => {
    expect(px(0)).toBe("0px");
    expect(px(8)).toBe("8px");
  });
});

describe("touch targets", () => {
  const controls = {
    "button.base": components.button.base,
    "input.base": components.input.base,
    "select.trigger": components.select.trigger,
    "select.item": components.select.item,
    "checkbox.row": components.checkbox.row,
    "tag.close": components.tag.close,
  };

  for (const [name, classes] of Object.entries(controls)) {
    it(`${name} keeps the minimum touch height`, () => {
      expect(classes.split(/\s+/)).toContain("min-h-touch");
    });
  }
});
