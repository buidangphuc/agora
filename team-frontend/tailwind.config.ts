import type { Config } from "tailwindcss";

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
const primary = {
  50: "#fff5f2",
  100: "#ffe8e1",
  200: "#ffd4c7",
  300: "#ffb5a0",
  400: "#ff8566",
  500: "#ee4d2d",
  600: "#d73211",
  700: "#b52309",
  800: "#941e0c",
  900: "#7a1d0f",
  950: "#430b05",
  DEFAULT: "#ee4d2d",
};

// Slate-free neutral: same values as Tailwind's `gray`, so existing gray-*
// utilities and the neutral-* tokens always agree.
const neutral = {
  50: "#f9fafb",
  100: "#f3f4f6",
  200: "#e5e7eb",
  300: "#d1d5db",
  400: "#9ca3af",
  500: "#6b7280",
  600: "#4b5563",
  700: "#374151",
  800: "#1f2937",
  900: "#111827",
};

const accent = {
  danger: "#d0011b",
  "danger-dark": "#b00016",
  promo: "#ffbe00",
  "promo-light": "#ffe97a",
  success: "#00bfa5",
  // Text-safe shades of promo / success for copy on their own 10% tints (WCAG AA on white).
  "promo-dark": "#8a5a00",
  "success-dark": "#007a69",
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
        accent,
        // Tier 2 (class names: bg-action-primary, text-text-primary, border-border-subtle, ...)
        "action-primary": alias("action-primary"),
        "action-primary-hover": alias("action-primary-hover"),
        "surface-page": alias("surface-page"),
        "surface-card": alias("surface-card"),
        "surface-muted": alias("surface-muted"),
        "border-subtle": alias("border-subtle"),
        "border-strong": alias("border-strong"),
        "text-primary": alias("text-primary"),
        "text-secondary": alias("text-secondary"),
        "text-disabled": alias("text-disabled"),
        "text-inverse": alias("text-inverse"),
        danger: alias("danger"),
        promo: alias("promo"),
        success: alias("success"),
        "focus-ring": alias("focus-ring"),
        // Legacy brand names (kept: referenced across features)
        brand: {
          DEFAULT: "#ee4d2d",
          dark: "#d73211",
          light: "#ffeee8",
          hover: "#f05d40",
        },
        mall: {
          DEFAULT: "#d0011b",
          dark: "#b00016",
        },
        shopee: {
          bg: "#f5f5f5",
          orange: "#ee4d2d",
          red: "#d0011b",
          yellow: "#ffbe00",
          green: "#00bfa5",
          dark: "#222222",
          gray: "#757575",
          border: "#e5e7eb",
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
  plugins: [],
};
export default config;
