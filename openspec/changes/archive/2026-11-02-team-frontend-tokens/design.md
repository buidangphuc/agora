## Context

See proposal.md. Today `tailwind.config.ts` defines Tier 1 colours by hand, `globals.css` defines the Tier 2 `:root` variables from them with `theme()`, and `tokens.test.ts` builds the CSS with the real config and asserts the orange. The platform exposes `primitives`, `resolveTheme(theme)` and `resolveShadows(theme)` from `@fe-platform/tokens` (vendored since A2), and its preset replaces `spacing`, `screens`, `fontSize` and `colors` entirely.

## Decisions

1. **Selective adoption, not `presets`.** Import `primitives` and `resolveTheme` in `tailwind.config.ts`; build `primary` from `primary-50..950`, `neutral` from `neutral-50..900`, accents from `danger`, `danger-dark`, `promo`, `promo-dark`, `success`, `success-dark` (keep the local `promo-light`). The preset's wholesale replacement would break default spacing steps and `xl:`/`2xl:` screens used across the app.
2. **`gray = neutral`.** The existing comment already promises that `gray-*` and `neutral-*` agree; keeping that promise moves ~250 raw `gray-*` uses to the cooler scale now, ahead of the B2 codemod.
3. **Tier 2 from a plugin.** A small inline Tailwind plugin calls `addBase({ ":root": { "--color-<name>": value, ..., "--shadow-card": ..., "--shadow-pop": ... } })` using `resolveTheme("light")` and `resolveShadows("light")`. All platform semantic names become `theme.colors` entries (`action-primary`, `surface-card`, `text-link`, `text-danger`, ...); the app's existing `danger`, `promo`, `success` aliases map to the platform primitives of the same name (`--color-danger`, `--color-promo`, `--color-success`). The dark block of `tokens.css` is never imported.
4. **Legacy groups recoloured, not removed:** `brand` = {DEFAULT: primary-600, dark: primary-700, light: primary-50, hover: primary-500}; `mall` = {DEFAULT: danger, dark: danger-dark}; `shopee` = {bg: neutral-100, orange: primary-600, red: danger, yellow: promo, green: success, dark: neutral-900, gray: neutral-500, border: neutral-200}. Class names are unchanged so call sites compile.
5. **Extras for later phases:** `boxShadow.card` and `boxShadow.pop` read the variables; `minHeight.touch` is 44px from `primitives.touchTarget`; `pt-safe`/`pb-safe` utilities as in the platform preset.
6. **globals.css:** remove the hand-written `:root` block; keep the base layer, scrollbar styles and the buy-bar rule (they reference `theme("colors.neutral.*")`, which still exists). Body font-size stays 14px.
7. **Tests:** the existing tokens test keeps its postcss-with-real-config approach. It now compares `bg-primary-500` to the platform primitive converted to `rgb(r g b`, `--color-action-primary` to `resolveTheme("light")["action-primary"]`, and adds: every platform semantic name defined on `:root` and exposed as a colour; the built CSS has no `prefers-color-scheme` or `data-theme` block; `gray` equals `neutral`; legacy groups contain none of the old orange/red/yellow/green hex values. e2e feature lines are updated to `rgb(99, 102, 241)` (primary-500) and `rgb(79, 70, 229)` / `#4f46e5` (action-primary).
8. **Verification beyond unit tests:** build the app, serve it, and read computed styles in a real browser (header button background, `--color-action-primary`), look at home, search, product and cart in the browser pane for leftover orange or unreadable text, and run the two updated e2e features if the harness can reach a locally started app.

## Risks / Trade-offs

- [Components that pair a raw palette tint with text could lose contrast under the new scale] -> `gray`=`neutral` keeps pairs like gray-100/gray-900 near-equal; a visual pass over the main pages is part of the verification and findings go to the report, not silently accepted.
- [`bg-primary-500` stays 500 (`#6366f1`) while `action-primary` is 600] -> same relationship the platform uses; white on 500 is below AA, so text on `bg-primary-500` would be flagged by the visual pass (only one use exists).
- [Seller and admin screens change colour too] -> intended; they split out later and inherit the same tokens.
- [Config imports an `.mjs` package] -> verified by running `next build` and the vitest build of the config.
