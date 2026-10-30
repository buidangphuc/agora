## Decisions

- Label folding (chosen by the user over a bare unlabelled platform input): the platform keeps `label` mandatory, so `FormItem label/help/status` around a single Input moves onto the Input (`label`, `helperText`, `error` when `status="error"`). `FormItem` stays for groups and non-Input controls.
- `inputSize` is dropped: every input meets the 44 px touch height. No call site used `leftIcon` or `rightIcon`, so the platform does not gain them.
- Required mark: rendered inside the label with `aria-hidden`, so the accessible name is unchanged; text-content label queries in tests must use a regex or role query.
- Checkbox moves from a native input to Radix: role, accessible name and `data-testid` are kept, so Playwright `check()` and `get_by_role("checkbox")` still work; inside a form Radix renders a hidden input so `name` still submits `on`.
- Mixed state uses a `checkbox.mixed` token applied by the component instead of an arbitrary `aria-[checked=mixed]` variant, which the tailwind content test cannot match.

## Risks

- Facade drops a caller `aria-describedby` (no call site used it).
- Inputs that had only an `aria-label` now show a visible label, which can change layout in tight rows.
