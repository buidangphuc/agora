## Why

The screens look like Shopee: the primary colour is Shopee's orange `#ee4d2d`, the page background is a flat grey, content is dense and corners are 8 to 12 px. The cause is P0, which copied `team-frontend`'s palette and radii as starting values. A template meant to make agents produce a coherent, current-looking UI should not inherit another brand's look.

The standard must also serve web, mobile web (WAP) and later a native app. Today token data carries web units (`"16px"`) and CSS-shaped values, and interactive controls are smaller than a touch target.

## What Changes

- New visual direction recorded in ADR-0005: indigo primary, near-white page, thin borders, soft shadows, large radii, more whitespace, larger body text.
- `packages/tokens`: new primary scale (indigo), cooler neutral scale, softer `promo`; radius step `3xl`; shadow tokens (`shadow-card`, `shadow-pop`) with light and dark values; semantic surfaces and borders re-pointed; every colour pair in `pairs.mjs` re-declared and checked in both themes.
- `packages/tokens`, platform-neutral data: raw numbers instead of unit strings for space, radius and font size; structured shadows (`x, y, blur, spread, color`); the CSS and Tailwind builders add units; a new committed `dist/tokens.json` with resolved light and dark values for future native builders. Token and class names do not change.
- `packages/tokens`, mobile web: `touch-target` (44), shared breakpoints (`sm 640`, `md 768`, `lg 1024`) exported to Tailwind `screens`, and `pt-safe`/`pb-safe` utilities from `env(safe-area-inset-*)`.
- `packages/ui`: class maps changed so Button, Input, Select, Checkbox, Table, Dialog, Toast, Link and the error panel use the new radii, shadows, spacing and type, and every interactive control is at least 44 px tall. Props, variants and sizes keep their names and values.
- `templates/app`: viewport export (`viewportFit: cover`); layouts of `/products`, `/shop`, `/shop/[id]`, `/shop/cart`, `/shop/checkout` and `/shop/confirmation` rebuilt mobile-first (airy grid 1 to 4 columns, two-column detail with sticky info, cart with sticky summary, stepped checkout, minimal confirmation, bottom action bar with safe-area padding). Server actions, validation, cookies and data loading are untouched.
- Docs: `docs/agents/frontend.md` gets the design rules and a cross-device section; `docs/tokens-starting-values.md` is marked historical; `catalog.json` regenerated with a `platforms` field per component; the consumer example's class check updated if class names change.
- Changesets for `tokens` and `ui` at the level the repository's own versioning policy requires (see design).

## Capabilities

### New Capabilities
none (visual design and platform-neutral token data of a template; no product behaviour)

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `packages/tokens/{src,scripts,dist}`, `packages/ui/src`, `templates/app/{tailwind.config.mjs,app/layout.tsx,app/(public)/**}`, `docs/`, `.changeset/`, `scripts/catalog.mjs`.
- Published `tokens` and `ui` change appearance for every consumer; `examples/consumer` must still build.
- No change to `api-client`, `team-frontend` or the backend.

## Non-goals

Anything native (Swift, Kotlin, Dart or React Native token builders, a `ui-native` package, Connect clients for native, an app shell, a WebView or PWA wrapper): this is a later phase (P8) and needs the app technology decided first. Also: dark-theme redesign beyond keeping every pair valid, new components beyond one shared card if the app hand-rolls cards, motion tokens, imagery, `(internal)` routes, renaming any token, and mapping into `team-frontend`.

## Non-functional requirements

- Contrast gate (WCAG AA text, 3:1 non-text) green in light and dark for every declared pair, including the new ones.
- No arbitrary Tailwind values, no hex outside `tokens.mjs`, no new disable comments.
- No horizontal overflow at 375, 768 and 1280 px; axe stays green; every interactive control at least 44 px tall.
- `dist/tokens.json` and `dist/tokens.css` agree on every value, enforced by a test.
