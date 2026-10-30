## Decisions

- Sort tabs: the shared `Tabs` pills variant is about 28 px high and lives in `src/components/ui`, which this change must not edit. The two link sorts are rendered as `Link` facades instead, keeping `aria-current="page"` and the link names the unit test and e2e use. The `nav` is labelled "Kiểu sắp xếp" because "Sắp xếp" is already the hidden label of the mobile select and a duplicate label breaks `getByLabelText`.
- Active filter pills: a `Tag` cannot hold a 44 px target without growing every tag, so each applied filter is a single `NextLink` pill whose `aria-label` is the old "Bỏ lọc <label>" name. The whole pill removes the filter, which is a larger target than the old "×".
- Tints: there is no semantic tint token for the active facet row, so it uses `bg-surface-muted` with `text-action-primary` instead of the raw `primary-50`.
- Saved search rows use `NextLink` with `text-text-link` because the `Link` facade forbids `className` and its button variants centre the text.

## Risks

- The numbered `Pagination` (from `sm` up) is a shared component with 36 px cells; it is left for the shared-component owners.
- `CategoryBar` pills and listing cards belong to `features/listing` and are measured but not changed here.
