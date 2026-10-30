## Decisions

- Facade keeps the Modal API: `isOpen`/`onClose` map to `open`/`onOpenChange(false)`, so call sites that pass a no-op `onClose` while a request is pending still keep the dialog open (controlled Radix root).
- Controlled without a trigger: Radix returns focus only to its own trigger, so the platform `Dialog` records the focused element in `onOpenAutoFocus` and restores it in `onCloseAutoFocus` when no `triggerLabel` is given.
- Native `dialog` element: Radix `Content` renders `asChild` onto `<dialog open>`, which keeps role `dialog`, the accessible name from the title and the `dialog[open]` query used by the ui-components focus-trap e2e step. `aria-modal="true"` is set explicitly; Radix itself only hides siblings.
- Description optional: Modal call sites without a description (about 8) would otherwise need invented copy; Radix only links `aria-describedby` when a description renders.
- Layout from tokens: `fixed inset-4 m-auto h-fit w-auto` plus a size max-width centres the dialog with a 16 px gutter and caps its height to the viewport; the body scrolls. No arbitrary values and no `min-h-0` (the platform preset has no such step; a scroll container's automatic minimum is already zero).
- Title: `title` becomes a required string because the platform requires a title; every call site already passed one, so the "Hộp thoại" fallback is gone.
- Tailwind content: team-frontend did not scan `@fe-platform/ui` or the tokens class maps, so platform-only utilities (for example `inset-4`, `bg-surface-overlay`, `aria-checked:bg-action-primary`) were missing from the CSS. The config now scans both, matching `fe-platform/examples/consumer`.

## Risks

- Scroll lock moves from `body.style.overflow` to `react-remove-scroll`; tests that asserted the inline style were Modal-only and were removed.
- Outside pointer events are disabled while a dialog is open, so the cookie banner is not clickable until it closes.
- Other C3 branches may add the same two Tailwind content lines; the integrator keeps one copy.
