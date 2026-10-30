## Why

C3 continues moving team-frontend's own UI components onto `@fe-platform/ui`. The team `Select` is a hand-styled native `<select>` used in 15 places. The platform `Select` is a Radix popup, but platform-e2e drives these fields with Playwright `select_option`, which only works on a real `<select>`. The platform therefore gains a native variant and the team `Select` becomes a facade over it.

## What Changes

- `@fe-platform/ui` and `@fe-platform/tokens` (minor): new `NativeSelect` export that renders a real `<select>` with the platform label (mandatory, optionally screen-reader only through `labelHidden`), required mark, help and error text, a disabled placeholder option and the 44 px touch height. The Radix `Select` API is unchanged. Tokens add `select.native` and `select.icon`.
- `team-frontend/src/components/ui/Select.tsx` becomes a facade over `NativeSelect` (`helperText` and `error` map to `helpText` and `errorText`; `label` is required; `className`, `selectSize`, `invalid` and `<option>` children are gone; `SelectOption.label` is a string).
- Selects labelled by a surrounding `FormItem` or a sibling `<label>` take the label, help and error text themselves; selects that only had an `aria-label` use `label` with `labelHidden` (sort bar) or a visible `label` (cancel reason, dev demos).
- Required selects whose `*` came only from `FormItem` (`Sản phẩm`, `Lý do trả hàng`, `Ngành hàng`) carry native `required`; their forms use `noValidate` or are not forms, so submit behaviour is unchanged.
- The listing category select builds its options from the category list with the old `-- Chọn ngành hàng --` text as a disabled placeholder.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src` (14 call-site files plus the facade and its tests), `team-frontend/vendor/*.tgz`, `team-frontend/package-lock.json`, `fe-platform/packages/{tokens,ui}`, `fe-platform/scripts/catalog.test.mjs`, `fe-platform/.changeset`, `fe-platform/docs/agents/catalog.json`.
- Visible changes: selects are at least 44 px high with the airy radius and a text chevron; labels use the platform label style; the cancel-reason select shows its label.

## Non-goals

Moving the platform Radix `Select` to call sites; admin native `<select>` elements that do not use the team `Select`; Table, Link, Dialog, Toast, Alert swaps; dark theme.

## Non-functional requirements

`npm run check` green in `fe-platform` and `team-frontend`; `npm run build` green; platform-e2e selectors keep working: `#role`, `#categoryId`, `data-testid="return-reason"`, `dialog.locator("select")` and `get_by_role("combobox", name="Loại giấy tờ")` all still resolve to a native `<select>`.
