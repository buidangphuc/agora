## Decisions

- Native variant as a separate export (`NativeSelect`) instead of a `native` prop on `Select`: the two have different event contracts (`onChange` on a DOM element versus `onValueChange`) and different client needs (`NativeSelect` is server-renderable, `Select` is a client component), so one prop would make the types a union and pull `"use client"` into every native use.
- `NativeSelect` takes `NativeProps<"select">` minus `children`, so `id`, `name`, `value`, `onChange`, `onBlur`, `required`, `aria-*` and `data-testid` reach the element and the e2e locators (`#role`, `#categoryId`, `return-reason`) keep working. Options come only from `options`; the one call site with `<option>` children (listing category) moves to `options` plus `placeholder`.
- Placeholder is a disabled first option with value `""`; an uncontrolled select with a placeholder defaults to it, matching the old team behaviour.
- `labelHidden` keeps an accessible name for the sort bar, whose visible text is the surrounding "Sắp xếp theo:" bar; the required mark is suppressed when the label is hidden.
- Chevron uses a text glyph in an `aria-hidden` span positioned by the `select.icon` token (`inset-y-0 right-4`), avoiding fractional or arbitrary Tailwind values.
- `FormItem` stays for groups and non-Select controls; its unit test now uses a plain `<select>` as the generic control.

## Risks

- `SelectOption.label` is a string now; no call site passed a node.
- The cancel-reason select in the order cancel dialog now shows a visible label, which adds one line to the dialog.
- The listing category placeholder is disabled, so a seller cannot reselect "none" once a category is picked.
