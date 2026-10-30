## Why

After B1 the buyer app's colours come from the platform, but 40 source files still name raw palette classes (`text-gray-900`, `bg-white`, `text-brand`, stock orange and indigo). Plan item B2 moves them to the semantic names the platform defines so a later dark theme, the seller and admin apps and the component swaps all read one vocabulary.

## What Changes

- A one-shot codemod rewrites class names in `team-frontend/src` (non-test code) using an explicit mapping, applied only where the platform has a matching name:
  - exact: `text-white`, `bg-white`, `text-brand`, `bg-brand`, `border-brand`, `bg-brand-dark`, `border-gray-200/300`, `text-gray-900`, `bg-gray-50/100`, `text-red-700`, `bg-red-600`, `text-emerald-700`, `bg-emerald-500`, the `indigo-N` scale (to `primary-N`) and `brand` with an opacity modifier (to `primary-600/N`);
  - near-exact (documented deltas): `text-gray-800` to `text-text-primary`, `text-gray-700/600/500` to `text-text-secondary`, `text-gray-400` to `text-text-disabled`, `border-gray-100` to `border-border-subtle`, `text-red-600` to `text-text-danger`, `bg-emerald-600` to `bg-accent-success-dark`, stock `orange-N` to `primary-N` (chat, assistant, buttons), `shadow-shopee` to `shadow-card`;
  - remaining `gray-N` becomes `neutral-N` (same colour), so no `gray-*` class is left.
- Classes inside a string that also sets a dark surface (`bg-slate-900`, `bg-neutral-900/50`, ...) and the whole admin cockpit are left alone: they are dark by design and the app has no dark theme yet.
- Classes with an opacity modifier on a semantic name are not converted, because the semantic colours are CSS variables without an alpha channel.
- New `src/app/palette.test.ts` fails when `brand`, `shopee`, `mall`, `gray-N`, `indigo-N` or (outside the flash-sale stock bar and the cockpit) `orange-N` classes come back.
- No component structure, spacing, radius or logic change.

## Capabilities

### New Capabilities
none (class names of an existing app)

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- 23 files in `team-frontend/src` (features, shell, ui facades) plus the new test.
- Visible colour changes only where listed under deltas in `design.md`.

## Non-goals

Removing the legacy `brand`/`mall`/`shopee`/`gray` Tailwind names (B3), tint tokens for success/danger/warning surfaces, alpha-capable semantic colours, dark theme, swapping components.

## Non-functional requirements

- Text that used `gray-500` gets darker (`text-secondary`), which only increases contrast; no foreground/background pair loses AA that had it.
- `npm run check` and `next build` stay green; the e2e colour scenarios from B1 stay green.
