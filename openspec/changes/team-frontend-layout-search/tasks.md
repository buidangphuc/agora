## 1. Layout

- [x] 1.1 Restyle the search page, loading skeleton and skeleton blocks (spacing, title scale, filter column width, semantic surface colour); verify the route tests pass
- [x] 1.2 Rebuild `SortBar`, `FilterSidebar`, `ActiveFilters`, `SavedSearches`, the results block and `CompactPagination` with 44 px targets and large radii; verify the `src/features/search` tests and `tsc` pass

## 2. Verification

- [x] 2.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 2.2 Load `/search` at 375, 768 and 1280 px against `next start`; verify no horizontal overflow and the page's controls are at least 44 px high
- [x] 2.3 `openspec validate team-frontend-layout-search --strict`
