## Context

See proposal.md. ADR-0001 fixed: wrapped headless primitives, token-driven Tailwind, versioned workspace packages. P2 provides `@fe-platform/tokens` (class maps for button, input, card; semantic Tailwind utilities; contrast gate). `team-frontend` has about 40 hand-built components and no headless library, so there is nothing to reuse by import.

## Goals / Non-Goals

**Goals:** seven components whose states and accessibility are verified by tests, built only from tokens, readable by an agent from the export list and prop types.
**Non-Goals:** Storybook, more components, form libraries, animation system.

## Decisions

1. **Radix primitives** (`@radix-ui/react-dialog|select|checkbox|toast`) as the headless layer. Alternatives: React Aria (heavier API, hook-based), Headless UI (no toast, fewer primitives). Radix covers all four interactive needs with unstyled parts and focus management included.
2. **Styling input is `variant` and `size` only.** No `className`/`style` pass-through on public props, so a screen cannot bypass tokens. Class strings come from `packages/tokens` component maps, extended there (select, dialog, toast, table, checkbox); a tiny `cx` helper joins them (no clsx dependency).
3. **Server/client split.** Button, Input, Table have no hooks and no `"use client"`. Select, Checkbox, Dialog, Toast start with `"use client"`. A vitest check scans the package: any file importing `@radix-ui/*` or using React hooks must start with the directive.
4. **State matrix as data.** Each component exports (from a test-only fixture file) the list of states it supports. Tests render every state and run axe-core, asserting zero violations; a deliberately inaccessible fixture (button with no accessible name) must fail, so the gate cannot go silently green.
5. **Small token additions** go through P2's gate: `border-danger` (error outline) and `surface-overlay` (dialog scrim, an 8-digit hex primitive). Error state is never colour alone: Input, Select and Checkbox set `aria-invalid` and accept an `errorText` rendered and linked with `aria-describedby`.
6. **jsdom + axe-core** for tests, driven through `@testing-library/react`. jsdom cannot judge colour contrast, so contrast stays the job of the P2 gate; axe runs with the colour-contrast rule off and says so in the test.
7. **Public API** is one entry file `src/index.ts` re-exporting components and their prop types; the P5 catalog export will read it, so each component documents purpose and variants in a typed JSDoc block.
8. **Source language** follows P2: plain `.mjs` is not viable for JSX, so `packages/ui` is TypeScript (`.tsx`) compiled by Next (`transpilePackages`) and by vitest; `tsc --noEmit` is the package typecheck.

## Risks / Trade-offs

- [Radix adds four runtime dependencies] → accepted for correct focus/aria; wrapped, so swappable.
- [axe in jsdom misses real-browser issues] → noted; real-browser checks wait for P5 Storybook/Playwright if wanted.
- [No `className` escape hatch frustrates one-off layouts] → layout belongs to the page via wrapper elements; add a variant when a second use appears.
- [`transpilePackages` coupling to Next] → documented in `docs/agents/project.md`.

## Migration Plan

Additive. Rollback: remove `packages/ui`, the template dashboard usage and the added tokens.

## Open Questions

none (Storybook deferred to P5, confirmed by the user 2026-10-29; state matrices cover the stories-per-state intent through tests).
