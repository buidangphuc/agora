## 1. Platform-neutral token data

- [x] 1.1 Convert `space`, `radius` and `fontSize` in `tokens.mjs` to numbers and add units in `resolve.mjs` and `preset.mjs`; verify `dist/tokens.css` and the generated class names are unchanged (test against a snapshot of names and unit values)
- [x] 1.2 Add structured shadow primitives and semantic `shadow-card` / `shadow-pop` for light and dark; CSS builder emits `box-shadow`; preset exposes them; verify a unit test resolves both themes
- [x] 1.3 Add `toJson()` and write `dist/tokens.json` from `scripts/build.mjs`; verify a test checks every colour, radius, space and shadow value in the JSON against `tokens.css`, and that the committed file has no drift
- [x] 1.4 Add a test that fails when source token data contains a platform unit string; verify it goes red when `"16px"` is put back, then green

## 2. Palette and shape

- [x] 2.1 Replace `primary-*` with the indigo scale, `neutral-*` with the cooler scale, soften `promo`; re-point `surface-page`, `surface-card` and borders; add `radius.3xl`
- [x] 2.2 Re-declare every pair in `pairs.mjs` (including link and shadow-adjacent surfaces); verify `npm run tokens` passes in light and dark, then lower one pair on purpose and verify the gate goes red before restoring it
- [x] 2.3 Rebuild `dist/tokens.css`, `dist/tokens.json` and `dist/types`; verify `tokens.test.mjs` drift tests pass

## 3. Mobile web tokens and base

- [x] 3.1 Add `touch-target`, breakpoints and `pt-safe`/`pb-safe` utilities to tokens and `preset.mjs`; verify the Tailwind content test and a preset test cover them
- [x] 3.2 Export `viewport` with `viewportFit: "cover"` from `templates/app/app/layout.tsx` and set the body text to `text-base`; verify `next build` passes

## 4. Component class maps

- [x] 4.1 Update `components` maps (button, field, input, select, checkbox, dialog, toast, table, link) to the new radii, shadows, spacing and type; every interactive control gets `min-h-touch`; verify `packages/ui` tests and axe stay green
- [x] 4.2 Add a class-map test that every interactive control class set contains `min-h-touch`; verify it goes red when one is removed
- [x] 4.3 Check whether the screens hand-roll a card pattern more than once; add a shared `Card` to `packages/ui` only if so, with a `states.tsx` entry and JSDoc purpose; otherwise record "no new component" in the change notes

## 5. Screens

- [x] 5.1 Rebuild `/products` and `/shop` (grid 1 to 4 columns, image-first cards, short hero); verify no logic or data-loading change in the diff
- [x] 5.2 Rebuild `/shop/[id]` (single column, two columns from `md` with sticky info) and `/shop/cart` (sticky summary from `md`, bottom bar with `pb-safe` on mobile)
- [x] 5.3 Rebuild `/shop/checkout` (stepped, one-column form, summary beside from `md`) and `/shop/confirmation`; verify server action, validation and cookie code are byte-identical
- [x] 5.4 In the browser pane capture all six screens at 375x812, 768 and desktop, light and dark; verify no horizontal overflow, controls at least 44 px, no orange left, and that restarting the dev server with `.next` cleared shows styled output

## 6. Agent layer and docs

- [x] 6.1 Add `platforms` to catalog entries in `scripts/catalog.mjs` from one constant; run `npm run catalog`; verify `catalog:check` is green and fails on a hand edit
- [x] 6.2 Write `docs/ADR/0005-visual-direction-and-platforms.md`, mark `docs/tokens-starting-values.md` historical, add the design rules and a cross-device section to `docs/agents/frontend.md`, note in `docs/experiments/p5-five-screens.md` that it was measured on the previous look; update the `fe-platform` skill and `AGENTS.md` if they mention colours or radii
- [x] 6.3 Verify `docs/agents/project.md` check order still matches `npm run check`

## 7. Governance and done-condition

- [x] 7.1 Add changesets: `tokens` major with a `## Migration` section (no names change, appearance changes), `ui` minor; verify `release:status` reports them
- [x] 7.2 Run `examples/consumer` against the new packages (install from the local registry only if it is running; otherwise against workspace builds) and verify its class check and build pass or record why it could not run
- [x] 7.3 Done-condition: `npm run check` exits 0 after a fresh `npm ci`, `npm run build -w templates/app` passes, no new disable comments, `npm audit` clean, `openspec validate fe-platform-visual-refresh --strict` passes
- [x] 7.4 e2e track: none (no new user-facing capability, no FEATURES.yaml entry); confirm `repo-doctor` reports no new drift in the fe-platform scope
