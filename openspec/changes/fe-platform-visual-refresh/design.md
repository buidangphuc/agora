## Context

See proposal.md. Current state: `packages/tokens/src/tokens.mjs` holds primitives (colour hex, space/radius/fontSize as strings such as `"16px"`), light and dark semantic maps pointing at primitives, and the `components` class maps that `packages/ui` consumes. `resolve.mjs` builds `dist/tokens.css`; `preset.mjs` feeds Tailwind; `pairs.mjs` declares the contrast pairs checked in both themes; `dist/` is committed and guarded by tests. Button `sm` is `px-3 py-1` (about 28 px high), below a touch target. `templates/app/app/layout.tsx` has no viewport export. App code may use only `@fe-platform/ui` and token utilities (boundary rules and the Tailwind gate), so most visual change lands in `tokens.mjs`.

The governance policy (`docs/governance/versioning.md`) calls pointing a semantic token at a different primitive family breaking. This change keeps primitive names (`primary-*`) but moves them from orange to indigo, so every consumer's brand colour changes.

## Goals / Non-Goals

**Goals:** a visibly different, current look (indigo, airy, large radii); one source of token data that is not tied to web units; mobile web done properly; a recorded decision (ADR-0005).
**Non-Goals:** anything native (P8), dark-theme redesign, motion, new components beyond a shared card, renaming tokens.

## Decisions

1. **Palette.** `primary-50..950` becomes an indigo scale (500 about `#6366f1`, 600 about `#4f46e5`, 700 about `#4338ca`, exact values chosen so every pair passes). `neutral-*` moves to a cooler slate scale. `promo` is toned down. `action-primary` stays `primary-600` in light; white on it must reach 4.5:1 (checked by the gate, not assumed). `surface-page` is near-white (`neutral-50`), `surface-card` white with `border-subtle`; dark keeps its mapping with re-checked pairs.
2. **Shape and depth.** Controls use `rounded-xl` (12), cards and dialogs `rounded-2xl` (16), hero `rounded-3xl` (24, new primitive); `xs` stays defined but no class map uses it. Shadows are two tokens: `shadow-card` (soft, low) and `shadow-pop` (menus, dialogs). Dark theme uses darker, lower-alpha shadows.
3. **Type and spacing.** Body text in content areas is 16 px (`text-base`); `text-sm` is for secondary text. Headings use `3xl`/`4xl` with tight tracking. Section rhythm uses existing space steps (`6`, `8`, `12`); no new arbitrary values.
4. **Platform-neutral token data.** In `tokens.mjs`, `space`, `radius` and `fontSize` hold numbers; `resolve.mjs` and `preset.mjs` append `px` (and `rem` where the preset already uses it) on output, so generated CSS and class names are unchanged. Shadows are arrays of `{ x, y, blur, spread, color }`; the CSS builder joins them into `box-shadow`. A new `toJson()` in `resolve.mjs` writes `dist/tokens.json` (resolved light and dark colours, numeric scales, structured shadows, breakpoints, `touch-target`). Alternative considered: Style Dictionary; rejected because the three outputs here are small and a new tool adds a dependency without removing code.
5. **Mobile web.** New primitives: `touch-target: 44`, `breakpoint: { sm: 640, md: 768, lg: 1024 }` (preset `screens`), and `pt-safe`/`pb-safe` plugin utilities using `env(safe-area-inset-top/bottom)`. Class maps give every interactive control (`button.size.*`, `input.base`, `select.trigger`, `checkbox.root` hit area) a minimum height of `touch-target` through a token-backed class (`min-h-touch`). `sm` keeps its name and a smaller visual padding but the same minimum height. The app root exports `viewport` with `viewportFit: "cover"`. Layouts are mobile-first; hover is never the only way to reach an action.
6. **Screens.** `/products` and `/shop`: 1, 2, 3, then 4 columns at `sm`, `md`, `lg`; image-first cards with `rounded-2xl` and `shadow-card`; short hero. `/shop/[id]`: single column on mobile, two columns from `md` with sticky info. `/shop/cart`: list plus a summary that is sticky from `md` and a bottom bar with `pb-safe` on mobile. `/shop/checkout`: stepped, one-column form, summary beside it from `md`. `/shop/confirmation`: minimal. A shared `Card` is added to `packages/ui` only if the screens hand-roll more than one card pattern; otherwise they reuse existing components and token utilities.
7. **Catalog.** `scripts/catalog.mjs` adds `platforms: ["web", "wap"]` to every component entry, derived from a single constant, so P8 can add `native` without changing the shape. `catalog:check` covers it.
8. **Versioning.** Per the written policy, moving the primary family and changing `action-primary` hue is breaking for `tokens`: a `major` changeset with a `## Migration` section stating that no class or token names change and that appearance changes (brand colour, radii, shadows). `ui` is `minor` (class maps changed, no API change, plus `min-h-touch` behaviour). The consumer example is re-checked against the new versions. Not publishing to the registry is part of this change; publishing stays a separate, explicit step.
9. **ADR-0005** records: leaving the borrowed palette; indigo and airy style; web and WAP share one codebase; native shares token JSON and contract but not components (decision on app technology left open for P8). `docs/tokens-starting-values.md` gains a note that it is historical.

## Risks / Trade-offs

- [Indigo values may fail contrast for some pairs] → pairs are declared first and the gate is run while choosing steps; adjust steps, never relax a threshold.
- [A major bump for a recolour] → consistent with the written policy; the Migration note says no code change is needed, which keeps the cost low.
- [Numeric units change generated CSS by accident] → a test compares the new `tokens.css` units and class output with a snapshot of names, and `tokens.json` is checked against `tokens.css` value by value.
- [Redesigned screens drift from what the five-screen experiment measured] → the experiment doc is not rewritten; it is marked as measured on the previous look.
- [44 px controls make dense tables look heavy] → Table rows are not interactive controls; only controls inside them are affected.

## Open Questions

- Native app technology (React Native, Swift/Kotlin, Flutter, WebView or PWA) decides the P8 scope; nothing in this change depends on the answer.
