## Why

B2 left no call site using the legacy colour names, so the compatibility layer B1 kept in `tailwind.config.ts` has no purpose and still lets a new `bg-brand` or `text-gray-500` compile. Plan item B3 removes it.

## What Changes

- `tailwind.config.ts` drops the `gray` alias and the `brand`, `mall` and `shopee` colour groups, and the `shopee`, `shopee-hover` and `shopee-card` shadows.
- `globals.css` drops the unused `.shopee-discount-tag` rule.
- `FloatingChatBubble` arrow uses `border-t-neutral-900` (the only `gray` use the B2 scan missed, a `border-t-` side class).
- `tokens.test.ts` asserts the legacy groups and shadows are absent instead of recoloured; `palette.test.ts` also matches side-specific border classes.
- The `Badge` variant named `mall` is a component API and stays.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/tailwind.config.ts`, `src/app/globals.css`, `src/components/ui/FloatingChatBubble.tsx`, `src/app/tokens.test.ts`, `src/app/palette.test.ts`.
- No rendered colour changes: the removed names had no users.

## Non-goals

Tint tokens, dark theme, component swaps, renaming the `Badge` `mall` variant.

## Non-functional requirements

`npm run check`, `next build` and the e2e colour scenarios stay green; the built CSS contains no `gray`, `brand` or `shopee` utility.
