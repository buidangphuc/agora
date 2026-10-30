## Why

C2 moved Input and Checkbox onto `@fe-platform/ui`. C3 does the same for the buyer app's `Modal`, the confirm and form dialog used by about 20 screens, so there is one dialog implementation with a 44 px close target, token-driven layout and Radix focus handling.

## What Changes

- `@fe-platform/ui` and `@fe-platform/tokens` (minor): `Dialog` can run controlled without a trigger (`triggerLabel` optional; focus returns to the element that opened it), `description` is optional, and it gains a `footer` slot, a `size` (`sm`/`md`/`lg`/`xl`) and a `closeLabel` for a header close button. It renders a native `dialog` element with `aria-modal="true"`. Tokens add the dialog header, body, footer, close button and size maps; the close button is in the touch-target test.
- `team-frontend/src/components/ui/Modal.tsx` becomes a facade over the platform `Dialog`, keeping the props `isOpen`, `onClose`, `title`, `description`, `footer`, `size` and `children`; `title` is now a required string; the close button keeps the name "Đóng".
- `useDialog.ts` stays because `Drawer` still uses it; `Drawer` is unchanged.
- `team-frontend/tailwind.config.ts` scans `@fe-platform/ui` sources and the tokens class maps, as the platform consumer example does, so platform component classes are generated.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src/components/ui/Modal.tsx` and its test, `team-frontend/tailwind.config.ts`, `team-frontend/vendor/*.tgz`, `team-frontend/package-lock.json`, `fe-platform/packages/{tokens,ui}`, `fe-platform/.changeset`, `fe-platform/docs/agents/catalog.json`.
- Call sites keep their props, so the 18 feature files and the dev catalogue compile unchanged.
- Visible changes: the overlay no longer blurs; the title is 20 px; the close button is 44 px; the backdrop is no longer a separate "Đóng" button (clicking it still closes).

## Non-goals

Drawer, Select, Table, Link, Toast, Alert swaps; replacing `useDialog`; dark theme; server actions or routes.

## Non-functional requirements

`npm run check` green in `fe-platform` and `team-frontend`; `npm run build` green; e2e selectors that use `get_by_role("dialog", name=...)`, buttons inside the dialog and `dialog[open]` keep working.
