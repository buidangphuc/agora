## Why

P10 section F rebuilds the main buyer pages of `team-frontend` on the `@fe-platform/ui` visual language: mobile-first, airy spacing, large radii, indigo actions, 44 px touch targets and no horizontal overflow. The search results page still used the pre-platform density: 36 px facet rows, 20 px filter-removal links, small sort pills and a cramped header.

## What Changes

- `src/app/(shop)/search/page.tsx` and `loading.tsx`: larger title (`text-2xl`/`md:text-3xl`), more vertical rhythm, a wider filter column (`lg:w-72`) and a wider column gap; skeletons use `bg-surface-muted` instead of the raw `neutral-200`.
- `SortBar`: a `rounded-2xl` card with `shadow-card`; the desktop "Liên quan" / "Mới nhất" tabs become `Link` facades (`primary` when active, `ghost` otherwise, `aria-current="page"` kept) inside a `nav` labelled "Kiểu sắp xếp", so each is a 44 px target.
- `FilterSidebar`: facet buckets are 44 px rows with `rounded-xl`, a 20 px tick box and `bg-surface-muted` for the active state (was the raw `primary-50`); groups get more spacing.
- `ActiveFilters`: each applied filter is one 44 px pill link (accessible name "Bỏ lọc ..." unchanged) instead of a `Tag` with a 20 px "×" link; "Xóa tất cả bộ lọc" is a `ghost` `Link`.
- `SavedSearches`: saved queries are 44 px row links; the header wraps on narrow screens.
- `SearchResultsBlock`: the retry link is a `secondary` button link, the empty state card is padded and "Xóa bộ lọc" is a `md` primary link; `CompactPagination` cells use `min-h-touch` and `rounded-xl`.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src/app/(shop)/search/*`, `team-frontend/src/features/search/{SearchBlocks,SortBar,FilterSidebar,ActiveFilters,SavedSearches,CompactPagination,SearchSkeletons}.tsx`, one class assertion in `loading.test.tsx`.
- Presentation only: no server action, data loading or URL logic changed; `data-testid`, roles, accessible names and headings used by unit tests and `platform-e2e/src/pages/search_page.py` are kept.

## Non-goals

Shared components (`src/components/ui/*`, `Pagination`, `Tabs`, `Breadcrumb`), the shell header with `SearchBar`, `CategoryBar` and `ListingGrid` from `features/listing`, fe-platform, tailwind config, dark theme.

## Non-functional requirements

`npm run check` and `npm run build` green in `team-frontend`; at 375, 768 and 1280 px the page has no horizontal overflow and the page's own controls are at least 44 px high.
