## Why

C1b moved Button and Card onto `@fe-platform/ui`. C2 does the same for Input and Checkbox, the next two duplicated controls, so the buyer app has one implementation of form fields with 44 px touch targets and the platform rule that every field has a label.

## What Changes

- `@fe-platform/ui` and `@fe-platform/tokens` (minor): `Input` shows an `aria-hidden` required mark; `Checkbox` accepts `checked="indeterminate"`, `labelHidden`, `id` and `data-testid`; tokens add `field.required` and `checkbox.mixed`.
- `team-frontend/src/components/ui/Input.tsx` becomes a facade (`helperText` and `error` map to platform `helpText` and `errorText`; `label` is required; `className`, `inputSize`, `leftIcon`, `rightIcon` are gone). `Checkbox.tsx` re-exports the platform Checkbox.
- Inputs labelled by a surrounding `FormItem` take the label, help and error text themselves and the `FormItem` is removed around them.
- Native checkboxes become Radix checkboxes: `onChange` becomes `onCheckedChange`, `aria-label` becomes `label` + `labelHidden`, and the cart shop box uses `checked="indeterminate"` instead of a ref.
- `vitest.setup.ts` stubs `ResizeObserver`, which the Radix form input needs under jsdom.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src` (about 20 files), `team-frontend/vendor/*.tgz`, `team-frontend/package-lock.json`, `team-frontend/vitest.setup.ts`, `fe-platform/packages/{tokens,ui}`, `fe-platform/.changeset`, `fe-platform/docs/agents/catalog.json`.
- Visible changes: inputs are at least 44 px high with the airy radius; small price-range inputs grow; inputs that only had an `aria-label` now show their label; checkboxes are 24 px Radix boxes with an indigo fill.

## Non-goals

Select, Table, Link, Dialog, Toast, Alert swaps; replacing `FormItem` around non-Input controls; dark theme; server actions or routes.

## Non-functional requirements

`npm run check` green in `fe-platform` and `team-frontend`; `npm run build` green; e2e selectors that use role, label or `data-testid` keep working; forms still submit checkbox values.
