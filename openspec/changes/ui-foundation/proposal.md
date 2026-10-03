## Why

`platform-core/docs/UI_SYSTEM_DESIGN.md` sets four principles (clarity, predictable commerce, token-first,
zero layout shift) and a 3-tier token model, but nothing enforces them. Today `team-frontend` has 9 raw hex
colours and 173 arbitrary Tailwind values (`text-[9px]`, `p-[7px]`, ...), the token tiers exist only as a
partial primary scale in `tailwind.config.ts`, and the quality gate `npm run check` is red (Biome 54 errors,
`tsc` 11 errors, Vitest 5 failures). No UI phase can be rebuilt on that base.

This change lays the foundation, modelled on Ant Design's token system (Seed → Map → Alias → Component,
the same three tiers as the doc) and its layout grid, without taking a runtime dependency on `antd`.

## What Changes

- Make `npm run check` green on the current tree (including the uncommitted design-system WIP).
- Define the three token tiers:
  - Tier 1 seed/primitive values in `tailwind.config.ts`: brand scale, neutral, danger/promo/success
    accents, 4px spacing, radius (8/12/16), elevation, type scale 12/14/16/20/24.
  - Tier 2 semantic aliases as CSS variables in `src/app/globals.css`, exposed as Tailwind colours
    (`action-primary`, `surface-card`, `surface-muted`, `border-subtle`, `text-primary`, `text-secondary`,
    `text-disabled`, `danger`, `promo`, `success`, `focus-ring`).
  - Tier 3 component tokens live inside the components (change `ui-core-components`) and reference Tier 2
    only.
- Add a token lint (`scripts/check-tokens.mjs`) that fails on raw hex, `rgb()` and arbitrary `-[...]` values
  in `src/**/*.tsx`, and append it to `npm run check`.
- Remove every existing violation (9 hex, 173 arbitrary values) by mapping to tokens.
- Add the app-level shells for the Ant Design Exception pattern: `not-found.tsx`, `error.tsx` and a root
  `loading.tsx` skeleton.

Repos: `team-frontend` only. Docs: `platform-core/docs/UI_SYSTEM_DESIGN.md` §2 gains the alias table.

## Capabilities

### New Capabilities
- `ui-design-tokens`: the 3-tier token contract and its lint.

### Modified Capabilities
- `frontend-quality`: the quality gate also runs the token lint and must be green.

## Non-goals

- No `antd` / `@ant-design/*` runtime dependency (the doc forbids it); Ant Design is a reference model only.
- No dark mode in this change (aliases are designed so it can be added later by redefining the variables).
- No component API changes (that is `ui-core-components`) and no page redesign (phase changes).
- No backend, gateway or proto change.

## Impact

- `team-frontend`: `tailwind.config.ts`, `src/app/globals.css`, `package.json` (`check` script),
  `scripts/check-tokens.mjs`, `src/app/{not-found,error,loading}.tsx`, and every file that currently holds
  a violation (top: `features/chat/ChatView.tsx`, `app/layout.tsx`, `features/admin/CockpitView.tsx`,
  `app/listing/[id]/page.tsx`).
- `team-frontend/FEATURES.yaml` and platform-e2e `frontend` features for the Exception pages.
