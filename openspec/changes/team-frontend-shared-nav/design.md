## Decisions

- Locale-neutral platform: navigation labels are props with English defaults. The team-frontend `Pagination` facade passes `Phân trang`, `Trang trước`, `Trang sau` and `Trang N`; Tabs, Breadcrumb and Stepper defaults (`Tabs`, `Breadcrumb`, `Progress`) already matched, so those facades are plain re-exports.
- Timeline states stay in the app: the platform has no Empty, Skeleton or ErrorState, so the facade handles `loading`, `error` and `emptyText` and the platform `Timeline` renders only the list.
- Server compatibility: `Tabs.tsx`, `Pagination.tsx`, `Breadcrumb.tsx`, `Stepper.tsx` and `Timeline.tsx` have no `use client`; only `TabsClient.tsx` is a client file, so the link variants still take functions from server components.
- Colours without alpha or primitives: dots use `bg-current` with a semantic text colour (`text-text-success`, `text-text-warning`, `text-border-strong`), replacing `bg-success`, `bg-promo` and the `/10` rings.
- Spacing without half steps: connectors use `border-l` and `border-t` instead of `w-0.5`/`h-0.5`, dots are `size-3` in a `size-6` box, and the underline tab list drops `-mb-px`.
- `className` removed from all five; the six call sites that used it wrap the component in a div with the same classes.
- Tailwind content: team-frontend did not scan `@fe-platform/ui`, so classes used only by the platform (`size-3`, `ring-surface-card`, `min-w-12` and others) were missing from the built CSS. The config now scans the vendored ui sources and `tokens.mjs`; every class in the token maps is present after `npm run build`.

## Risks

- Visual shifts listed in the proposal (touch heights, connector style, warning dot colour) are unchecked in a browser.
- Merge conflicts with the other D batches in `index.ts`, `states.tsx`, `ui.test.tsx`, `catalog.test.mjs`, `tokens.mjs` and `tailwind.config.ts` (additive).
