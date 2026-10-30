## Why

P2 gave tokens and gates, but an agent still has no components to compose: it would hand-roll buttons and dialogs with their own focus and aria handling, which is where UI drifts and a11y breaks. A small shared kit built on headless primitives gives agents one correct way to build each control.

## What Changes

- New `fe-platform/packages/ui` with seven components: Button, Input, Select, Checkbox, Dialog, Toast, Table.
- Interactive pieces (Select, Checkbox, Dialog, Toast) wrap Radix headless primitives (decision 3 of ADR-0001); Button, Input and Table are native elements. All styling comes from the token class maps in `packages/tokens`.
- Every component has a declared state matrix (default, hover, focus-visible, disabled, plus loading and error where they apply) used by tests; each is rendered and checked with axe.
- Components that need browser APIs carry `"use client"`; Button, Input and Table stay server-safe. A test fails if a client-only dependency appears in a file without the directive.
- `packages/tokens` gains the few tokens the kit needs (`border-danger`, `surface-overlay`), covered by the existing contrast gate.
- `templates/app` renders one of each on the internal dashboard route to prove the package builds under Next.
- No change to `team-frontend`.

## Capabilities

### New Capabilities
none (shared UI library; no service or product behaviour)

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- New workspace package `packages/ui`; `templates/app` depends on it.
- New dependencies: `@radix-ui/react-dialog`, `-select`, `-checkbox`, `-toast`; dev: `@testing-library/react`, `jsdom`, `axe-core`. The `@testing-library/jest-dom` matchers are not added; assertions use plain DOM.
- Root `npm run check` is unchanged in shape; the new tests run inside `vitest run`.

## Non-goals

Storybook and Storybook MCP (P5, optional); components beyond the seven; theming beyond light/dark; data-fetching components; mapping into `team-frontend`.

## Non-functional requirements

- `npm run check` stays under 60 s locally with the kit included.
- No component takes a `className` or `style` prop that bypasses tokens (variants and sizes are the only styling inputs).

## Data classification

| Data element | Class | Retention | Controls |
|---|---|---|---|
| Component source | public | n/a | none beyond repo access |

No personal data handled.

## Threat model

none - no new trust boundary
