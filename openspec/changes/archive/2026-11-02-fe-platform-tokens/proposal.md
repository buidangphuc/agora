## Why

`fe-platform` has an enforced empty workspace (P1) but no design tokens, so agent-generated UI still has nothing to be consistent with and nothing stops raw hex or arbitrary Tailwind values. Tokens are the base every later package (ui, catalog) depends on, so they come next.

## What Changes

- New `fe-platform/packages/tokens`: three tiers (primitives, semantic aliases, component class maps), light and dark themes, authored in code from `docs/tokens-starting-values.md`.
- Build output: CSS custom properties per theme and a Tailwind preset that exposes only token-backed utilities.
- Token check script, wired into `npm run check`: fails on raw hex/rgb colours and Tailwind arbitrary values (`[...]`) in app and package source, with the `tokens-allow: <reason>` escape hatch.
- WCAG contrast gate: declared text/background pairs per theme, large-text declared explicitly; fails when a pair is below its threshold.
- Resolves the known issue: white on `#ee4d2d` is about 3.66:1.
- `templates/app` consumes the preset and CSS variables.
- No change to `team-frontend`.

## Capabilities

### New Capabilities
none (tooling and design data only; no service or user-facing behaviour)

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- New workspace package `packages/tokens`; `templates/app` gains Tailwind config and a CSS import; root `npm run check` gains two steps.
- New dev dependencies: `tailwindcss` (preset target). Contrast maths is hand-written, no library.
- Follow-ups unchanged: P3 ui, P4 api-client and reference feature, P5 agent layer, P6 governance.

## Non-goals

Components (P3); themes beyond light/dark; design-tool sync; mapping into `team-frontend`; runtime theme switching UI.

## Non-functional requirements

- Token build and the two new checks add under 10 s to `npm run check`.
- Build output is deterministic; regenerating produces no diff.

## Data classification

| Data element | Class | Retention | Controls |
|---|---|---|---|
| Colour, type, spacing values | public | n/a | none beyond repo access |

No personal data handled.

## Threat model

none - no new trust boundary
