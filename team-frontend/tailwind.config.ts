import { primitives, resolveShadows, resolveTheme } from "@fe-platform/tokens";
import type { Config } from "tailwindcss";
import plugin from "tailwindcss/plugin";

/**
 * Design tokens (see platform-core/docs/UI_SYSTEM_DESIGN.md section 2).
 *
 *  Tier 1  Primitives  - declared HERE and nowhere else (raw values live in this file only).
 *  Tier 2  Aliases     - CSS variables in src/app/globals.css, exposed below as Tailwind colours.
 *  Tier 3  Components  - class maps inside src/components/ui/*, referencing Tier 2 only.
 *
 * Component code must not contain raw hex / rgb() / arbitrary `-[...]` values
 * (enforced by scripts/check-tokens.mjs).
 */

// ---------------------------------------------------------------- Tier 1
// Colours come from @fe-platform/tokens (ADR-0005, ADR-0006); only the layout sizes, radii
// and type scale below are still local. The platform preset is not applied wholesale because
// it replaces spacing, screens and font sizes that this app still uses.
const palette = primitives.color as Record<string, string>;

const scale = (prefix: string, steps: number[]): Record<string, string> =>
  Object.fromEntries(steps.map((n) => [n, palette[`${prefix}-${n}`]]));

const primary: Record<string, string> = {
  ...scale("primary", [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950]),
  DEFAULT: palette["primary-600"],
};

// `gray` is aliased to this scale below, so existing gray-* utilities and the neutral-* tokens
// always agree.
const neutral = scale(
  "neutral",
  [50, 100, 200, 300, 400, 500, 600, 700, 800, 900],
);

const accent = {
  danger: palette.danger,
  "danger-dark": palette["danger-dark"],
  promo: palette.promo,
  "promo-light": "#ffe97a",
  success: palette.success,
  // Text-safe shades of promo / success for copy on their own 10% tints (WCAG AA on white).
  "promo-dark": palette["promo-dark"],
  "success-dark": palette["success-dark"],
};

// Type scale is limited to 12 / 14 / 16 / 20 / 24 px. Tailwind's larger step
// names are kept as aliases of the nearest allowed size so no class resolves
// to an off-scale (or sub-12px) font size.
const fontSize: Record<string, [string, { lineHeight: string }]> = {
  xs: ["0.75rem", { lineHeight: "1rem" }],
  sm: ["0.875rem", { lineHeight: "1.25rem" }],
  base: ["1rem", { lineHeight: "1.5rem" }],
  lg: ["1.25rem", { lineHeight: "1.75rem" }],
  xl: ["1.25rem", { lineHeight: "1.75rem" }],
  "2xl": ["1.5rem", { lineHeight: "2rem" }],
  // Display sizes: hero and page-title headings only.
  "3xl": ["1.875rem", { lineHeight: "2.25rem" }],
  "4xl": ["2.25rem", { lineHeight: "2.5rem" }],
  "5xl": ["2.25rem", { lineHeight: "2.5rem" }],
};

// ---------------------------------------------------------------- Tier 2 (colour aliases)
const alias = (name: string) => `var(--color-${name})`;

const lightTheme = resolveTheme("light");
const platformAliases = Object.fromEntries(
  Object.keys(lightTheme).map((name) => [name, alias(name)]),
);

// Tier 2 variables and platform utilities. Only the light theme is emitted: the app has no
// dark design and its components still use raw palette utilities.
const platformTokens = plugin(({ addBase, addUtilities }) => {
  const vars: Record<string, string> = {};
  for (const [name, value] of Object.entries(lightTheme)) {
    vars[`--color-${name}`] = value;
  }
  vars["--color-danger"] = palette.danger;
  vars["--color-promo"] = palette.promo;
  vars["--color-success"] = palette.success;
  for (const [name, value] of Object.entries(resolveShadows("light"))) {
    vars[`--shadow-${name}`] = value;
  }
  addBase({ ":root": vars });
  addUtilities({
    ".pt-safe": { paddingTop: "env(safe-area-inset-top)" },
    ".pb-safe": { paddingBottom: "env(safe-area-inset-bottom)" },
  });
});

const config: Config = {
  content: [
    "./src/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/app/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/components/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/features/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  theme: {
    fontSize,
    extend: {
      colors: {
        // Tier 1
        primary,
        neutral,
        gray: neutral,
        accent,
        // Tier 2 (class names: bg-action-primary, text-text-primary, border-border-subtle, ...):
        // every platform semantic name, plus the three app aliases below.
        ...platformAliases,
        danger: alias("danger"),
        promo: alias("promo"),
        success: alias("success"),
        // Legacy names (kept so call sites compile; recoloured to the platform palette, removed
        // in a later change)
        brand: {
          DEFAULT: primary[600],
          dark: primary[700],
          light: primary[50],
          hover: primary[500],
        },
        mall: {
          DEFAULT: palette.danger,
          dark: palette["danger-dark"],
        },
        shopee: {
          bg: neutral[100],
          orange: primary[600],
          red: palette.danger,
          yellow: palette.promo,
          green: palette.success,
          dark: neutral[900],
          gray: neutral[500],
          border: neutral[200],
        },
      },
      fontFamily: {
        sans: [
          "Inter",
          "-apple-system",
          "BlinkMacSystemFont",
          '"Segoe UI"',
          "Roboto",
          '"Helvetica Neue"',
          "Arial",
          '"Noto Sans"',
          "sans-serif",
          '"Apple Color Emoji"',
          '"Segoe UI Emoji"',
          '"Segoe UI Symbol"',
        ],
      },
      // 4px baseline is Tailwind's default spacing; 18 (72px) is the only addition.
      spacing: {
        18: "4.5rem",
      },
      // lg 8px (controls), xl 12px (cards), 2xl 16px (modal / hero); xs 2px for hairline chips.
      borderRadius: {
        xs: "0.125rem",
        lg: "0.5rem",
        xl: "0.75rem",
        "2xl": "1rem",
      },
      boxShadow: {
        card: "var(--shadow-card)",
        pop: "var(--shadow-pop)",
        "2xs": "0 1px 0 0 rgba(0, 0, 0, 0.05)",
        xs: "0 1px 2px 0 rgba(0, 0, 0, 0.05)",
        "preline-card":
          "0 1px 3px 0 rgba(0, 0, 0, 0.07), 0 1px 2px -1px rgba(0, 0, 0, 0.05)",
        "preline-hover":
          "0 10px 15px -3px rgba(0, 0, 0, 0.08), 0 4px 6px -4px rgba(0, 0, 0, 0.04)",
        shopee: "0 1px 1px 0 rgba(0, 0, 0, 0.05)",
        "shopee-hover": "0 2px 8px 0 rgba(0, 0, 0, 0.12)",
        "shopee-card":
          "0 1px 2px 0 rgba(60,64,67,.1), 0 1px 3px 1px rgba(60,64,67,.05)",
      },
      // Layout sizes that replaced arbitrary values: 1200px page container, chat
      // bubble widths, viewport-relative page/chat panes and the AI modal height.
      maxWidth: {
        page: "75rem",
        bubble: "75%",
        "bubble-wide": "85%",
      },
      height: {
        chat: "calc(100vh - 280px)",
        modal: "37.5rem",
      },
      minHeight: {
        touch: `${primitives.touchTarget}px`,
        "viewport-main": "calc(100vh - 140px)",
        chat: "32.5rem",
      },
      maxHeight: {
        chat: "42.5rem",
      },
      backdropBlur: {
        xs: "2px",
      },
      dropShadow: {
        xs: "0 1px 1px rgba(0, 0, 0, 0.05)",
        // Legible text over hero imagery (replaces an arbitrary drop-shadow value).
        "on-image": "0 1px 2px rgba(0, 0, 0, 0.6)",
      },
      aspectRatio: {
        "2/1": "2 / 1",
        "4/3": "4 / 3",
        "3/4": "3 / 4",
      },
    },
  },
  plugins: [platformTokens],
};
export default config;
