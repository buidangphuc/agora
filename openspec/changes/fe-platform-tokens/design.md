## Context

See proposal.md. P0 recorded starting values in `fe-platform/docs/tokens-starting-values.md` (copied from `team-frontend`); authority moves to `packages/tokens` here. P1 gave `npm run check` (biome, tsc, boundaries, disables, vitest).

## Goals / Non-Goals

**Goals:** one code source of truth for tokens; generated CSS and Tailwind preset; two machine gates (token usage, contrast) that agents cannot talk their way around.
**Non-Goals:** components, runtime theme toggle UI, syncing with `team-frontend`.

## Decisions

1. **Source is a plain ESM module with JSDoc types** (`src/tokens.mjs`) with `primitives`, `semantic.light`, `semantic.dark`, `components`. Semantic values reference primitive names, never raw values. Plain JS because Node 22.14 cannot run TS without a loader and a loader is a new tool; `tsc` still checks it through `checkJs`. Style Dictionary rejected: new tool, no benefit at this size.
2. **Build script** (`scripts/build.mjs` in the package) emits only `dist/tokens.css` (`:root`, `[data-theme="dark"]`, `prefers-color-scheme` fallback). `dist/` is committed so regeneration drift shows in review, and a test fails if a rebuild differs. The Tailwind preset is the source module itself (`src/preset.mjs`), not a generated file.
3. **Tailwind preset replaces, not extends,** colour, spacing, radius and fontSize theme keys, so only token-backed utilities exist. Colours expose semantic tokens only (never primitives) and map to `var(--...)`.
4. **Token check** (`scripts/check-tokens.mjs` at repo root, by copy of the `team-frontend` approach) scans `packages/*/src` (except `tokens`) and `templates/*` for hex/rgb/hsl literals and `[...]` arbitrary Tailwind values; escape hatch `tokens-allow: <reason>` is already counted by the disables check.
5. **Contrast gate** (`packages/tokens/src/contrast.mjs` plus a vitest suite). Pairs are declared in `src/pairs.mjs` as `{fg, bg, theme, size: "normal" | "large" | "ui"}`; thresholds 4.5 and 3. Tests iterate every declared pair in both themes and also assert that a deliberately bad pair is reported as failing, so the gate cannot go silently green. Non-text UI pairs (focus ring) use 3.
6. **Brand colour resolution:** keep primary 500 `#ee4d2d` as a primitive, but `action-primary` (a fill that carries white text) maps to primary 600 `#d73211`, which computes to about 4.8:1 with white. Alternative: keep 500 and declare it large-text only; rejected because buttons use normal-size text.
7. **Dark theme** is defined here: surfaces from neutral 900/800, text from neutral 50/300, action-primary stays primary 600; every pair verified by the gate, not by eye. Where the gate rejects a source value (focus-ring primary 300 is about 1.7:1 on white), the semantic alias moves to a passing primitive.

## Risks / Trade-offs

- [Primary button is darker than the current brand red] → recorded here; user can override with a one-line token change and the gate says if it still passes.
- [Committed `dist/` can go stale] → rebuild-equals-committed test in `check`.
- [Token check false positives on legitimate literals such as SVG attributes] → `tokens-allow: <reason>` with a mandatory reason.
- [Hand-written contrast maths wrong] → tests pin known ratios (black/white 21:1, `#ee4d2d` on white 3.66:1).

## Migration Plan

Additive. Rollback: remove `packages/tokens` and the two check steps.

## Open Questions

none (decision 6 confirmed by the user 2026-10-29).
