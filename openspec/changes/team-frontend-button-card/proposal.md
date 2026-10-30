## Why

C1a gave `@fe-platform/ui` a Button and Card that can stand in for `team-frontend`'s own. C1b makes `team-frontend` use them: the buyer app gets the indigo, airy controls and one implementation of both components, while the platform ban on `className` and `style` stays.

## What Changes

- `@fe-platform/ui` and `@fe-platform/tokens` (minor): `Button` `fullWidth` also accepts `"mobile"` (`w-full sm:w-auto`), needed by 9 call sites.
- `team-frontend/src/components/ui/Button.tsx` and `Card.tsx` become facades over the platform: same exports and `isLoading`, `aria-disabled` kept; variants `outline` and `white` map to platform `secondary`; `className` is no longer accepted; the Card facade defaults to flush (`padded` false) so unchanged call sites keep their layout.
- Every call site that passed `className` to a Button, Card or Card part is rewritten to semantic props (`fullWidth`, `variant`, `padded`) or to a plain wrapper element for placement and inner layout.
- Tests that asserted old class output are updated; behaviour assertions are unchanged.
- Tarballs and the `team-frontend` lockfile integrity are refreshed.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src` (about 45 files), `team-frontend/vendor/*.tgz`, `team-frontend/package-lock.json`, `fe-platform/packages/{tokens,ui}`, `fe-platform/.changeset`, `fe-platform/docs/agents/catalog.json`.
- Visible changes: every button is at least 44 px high (13 `xs` and 82 `sm` buttons grow); `outline` and `white` render as bordered `secondary`; destructive text buttons (`text-danger`) become solid `danger`; the loading spinner sits beside the label instead of overlaying it, so width can change while loading; card radius, shadow and border follow the airy tokens; hover no longer lifts a card.

## Non-goals

Input, Checkbox, Table, Link, Select, Dialog, Toast, Alert swaps; dark theme in `team-frontend`; changing server actions or routes.

## Non-functional requirements

`npm run check` green in `fe-platform` and `team-frontend`; the colour e2e scenarios stay green; no `className` reaches a platform component.
