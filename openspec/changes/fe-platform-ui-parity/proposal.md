## Why

Plan item C swaps `team-frontend`'s own Button and Card for `@fe-platform/ui`. Today the platform versions cannot stand in: Button lacks `danger`, `outline`, `white`, size `xs`, icons and a full-width option; Card has no `hoverable`, `loading`, flush padding or sub-parts, which `team-frontend` uses 249 times. The platform rule that bans `className` and `style` stays, so every missing capability becomes a semantic prop or token.

## What Changes

- `@fe-platform/tokens` (minor): semantic `action-danger` and `action-danger-hover` (light and dark), primitive `danger-darker`, contrast pairs for them, and class-map entries for the new button and card parts.
- `@fe-platform/ui` (minor): `Button` gains variant `danger`, size `xs`, `fullWidth`, `leftIcon`, `rightIcon` and a spinner while `loading`; `Card` gains `padded`, `hoverable`, `loading`; new `CardHeader`, `CardTitle`, `CardDescription`, `CardContent`, `CardFooter`.
- State matrix, typecheck fixtures, unit tests and the generated catalog cover each addition; `className` and `style` stay rejected on all of them.
- `npm run vendor` refreshes the `team-frontend` tarballs so the platform gate (`vendor:check`) stays green; `team-frontend` source is untouched. The next change (C1b) swaps the call sites.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `fe-platform/packages/tokens/src` and `dist`, `fe-platform/packages/ui/src`, `fe-platform/.changeset`, `fe-platform/docs/agents/catalog.json`, `team-frontend/vendor/*.tgz`.
- Existing platform props, variants and class output for current usages are unchanged except `Card`, whose padding moves from the base class to `padded` (default on, so the rendered result is the same).

## Non-goals

Replacing `team-frontend` call sites, an `outline` or `white` variant (`secondary` covers both in C1b), dark-theme work in `team-frontend`, Input, Table and the other swaps.

## Non-functional requirements

`npm run check` in `fe-platform` exits 0 (contrast gate included); axe finds no violation on any new state; control height stays at least 44 px.
