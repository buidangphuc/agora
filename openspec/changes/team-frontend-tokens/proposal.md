## Why

`team-frontend` still carries the borrowed Shopee palette (orange `#ee4d2d`, Tailwind-gray neutrals, red/yellow/green accents) that ADR-0005 retired in the platform. Plan item B1 moves the buyer app's colours onto the fe-platform primitives so every screen picks up the new identity without touching a component, and so later changes (component swaps, new apps) start from one source of colour.

## What Changes

- `team-frontend/tailwind.config.ts` takes the Tier 1 colours from `@fe-platform/tokens`: `primary` (indigo), `neutral` (cooler slate), the accent colours, and `gray` is made equal to `neutral` so the ~250 `gray-*` uses stay consistent with the tokens. `primary.DEFAULT` becomes `primary-600` (white text passes AA on it).
- Tier 2 aliases come from the platform: a Tailwind plugin writes `:root { --color-* }` from `resolveTheme("light")` (all platform semantic names, which include the 15 names the app already uses) and `--shadow-card` / `--shadow-pop`; the hand-written `:root` block in `globals.css` is removed. Only the light theme is emitted, because the app has no dark design and its components still use raw palette utilities.
- Legacy `brand`, `mall` and `shopee` colour groups keep their class names but are recoloured to the new palette (no orange left); they are removed later in B3.
- New utilities `shadow-card`, `shadow-pop`, `min-h-touch` and `pt-safe` / `pb-safe` become available for the coming component work.
- The platform preset is NOT applied wholesale: it replaces spacing, screens and font sizes, and the app uses default Tailwind spacing steps (`p-1.5`, `h-40`) and `xl:` screens.
- Tests: `src/app/tokens.test.ts` asserts against the platform primitives instead of literal orange values and gains checks (all platform semantic names exposed, no dark block, `gray` equals `neutral`, no orange left in legacy groups). The three e2e feature lines that pin `rgb(238, 77, 45)` / `#ee4d2d` move to the new brand values.
- No component, page, spacing, radius or type change.

## Capabilities

### New Capabilities
none (visual tokens of an existing app)

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/tailwind.config.ts`, `team-frontend/src/app/globals.css`, `team-frontend/src/app/tokens.test.ts`.
- `platform-e2e/tests/e2e/features/frontend/ui_foundation.feature` and `ui_token_gates.feature` (expected colour values).
- Every buyer, seller and admin screen in `team-frontend` changes colour at once.

## Non-goals

Dark theme, replacing components, radius/spacing/type changes, removing legacy colour names (B3), the raw-palette codemod (B2), changing `scripts/check-tokens.mjs`.

## Non-functional requirements

- Text and control colours that carried AA contrast keep it: `action-primary` with white text and body text on `surface-*` come from pairs the platform already gates in both themes; `promo` and `success` text uses the platform's dark shades.
- `npm run check` and `next build` stay green; the Docker build from `team-frontend/` alone still passes.
